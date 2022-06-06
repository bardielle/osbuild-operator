package manifests

import (
	"context"
	"fmt"
	_ "github.com/golang/mock/mockgen/model"
	"github.com/project-flotta/osbuild-operator/internal/customizations"
	"github.com/project-flotta/osbuild-operator/internal/repository/configmap"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfigtemplate"
	"github.com/project-flotta/osbuild-operator/internal/templates"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/project-flotta/osbuild-operator/api/v1alpha1"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate mockgen -package=manifests -source=osbuild.go -destination=mock_osbuildcrcreator.go
type OSBuildCRCreator interface {
	Create(ctx context.Context, osBuildConfig *v1alpha1.OSBuildConfig,
		osBuildConfigRepository osbuildconfig.Repository, osBuildRepository osbuild.Repository,
		osBuildConfigTemplateRepository osbuildconfigtemplate.Repository, configMapRepository configmap.Repository,
		scheme *runtime.Scheme) error
}
type OSBuildCreator struct{}

var zero int

func NewOSBuildCRCreator() OSBuildCRCreator {
	return &OSBuildCreator{}
}

func (o *OSBuildCreator) Create(ctx context.Context, osBuildConfig *v1alpha1.OSBuildConfig,
	osBuildConfigRepository osbuildconfig.Repository, osBuildRepository osbuild.Repository,
	osBuildConfigTemplateRepository osbuildconfigtemplate.Repository, configMapRepository configmap.Repository,
	scheme *runtime.Scheme) error {
	logger := log.FromContext(ctx)

	lastVersion := osBuildConfig.Status.LastVersion
	if lastVersion == nil {
		lastVersion = &zero
	}
	osBuildNewVersion := *lastVersion + 1

	osBuildName := fmt.Sprintf("%s-%d", osBuildConfig.Name, osBuildNewVersion)
	osBuild := &v1alpha1.OSBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      osBuildName,
			Namespace: osBuildConfig.Namespace,
		},
		Spec: v1alpha1.OSBuildSpec{
			TriggeredBy: "UpdateCR",
		},
	}

	osBuildConfigSpecDetails := osBuildConfig.Spec.Details.DeepCopy()
	kickstartConfigMap, osConfigTemplate, err := o.applyTemplate(ctx, osBuildConfig, osBuildConfigSpecDetails, osBuildName, osBuild, osBuildConfigTemplateRepository, configMapRepository)
	if err != nil {
		logger.Error(err, "cannot apply template to osBuild")
		return err
	}
	osBuild.Spec.Details = *osBuildConfigSpecDetails

	// Set the owner of the osBuild CR to be osBuildConfig in order to manage lifecycle of the osBuild CR.
	// Especially in deletion of osBuildConfig CR
	err = controllerutil.SetControllerReference(osBuildConfig, osBuild, scheme)
	if err != nil {
		logger.Error(err, "cannot create osBuild")
		return err
	}

	patch := client.MergeFrom(osBuildConfig.DeepCopy())
	osBuildConfig.Status.LastVersion = &osBuildNewVersion
	if osConfigTemplate != nil {
		osBuildConfig.Status.CurrentTemplateResourceVersion = &osConfigTemplate.ResourceVersion
		osBuildConfig.Status.LastTemplateResourceVersion = &osConfigTemplate.ResourceVersion
	}
	err = osBuildConfigRepository.PatchStatus(ctx, osBuildConfig, &patch)
	if err != nil {
		logger.Error(err, "cannot update the field lastVersion of osBuildConfig")
		return err
	}

	err = osBuildRepository.Create(ctx, osBuild)
	if err != nil {
		logger.Error(err, "cannot create osBuild")
		return err
	}

	if kickstartConfigMap != nil {
		err = o.setKickstartConfigMapOwner(ctx, kickstartConfigMap, osBuild, scheme, configMapRepository)
		if err != nil {
			logger.Error(err, "cannot set controller reference to kickstart config map")
			return err
		}
	}

	logger.Info("A new OSBuild CR was created", "OSBuild", osBuild.Name)

	return nil
}

func (o *OSBuildCreator) applyTemplate(ctx context.Context, osBuildConfig *v1alpha1.OSBuildConfig, osBuildConfigSpecDetails *v1alpha1.BuildDetails, osBuildName string, osBuild *v1alpha1.OSBuild, osBuildConfigTemplateRepository osbuildconfigtemplate.Repository, configMapRepository configmap.Repository) (*corev1.ConfigMap, *v1alpha1.OSBuildConfigTemplate, error) {
	var kickstartConfigMap *corev1.ConfigMap
	var osConfigTemplate *v1alpha1.OSBuildConfigTemplate
	if template := osBuildConfig.Spec.Template; template != nil {
		var err error
		osConfigTemplate, err = osBuildConfigTemplateRepository.Read(ctx, template.OSBuildConfigTemplateRef, osBuildConfig.Namespace)
		if err != nil {
			return nil, nil, err
		}

		osBuildConfigSpecDetails.Customizations = customizations.MergeCustomizations(osConfigTemplate.Spec.Customizations, osBuildConfigSpecDetails.Customizations)

		kickstartConfigMap, err = o.createKickstartConfigMap(ctx, osBuildConfig, osConfigTemplate, configMapRepository, osBuildName, osBuild.Namespace)
		if err != nil {
			return nil, nil, err
		}
		if kickstartConfigMap != nil {
			osBuild.Spec.Kickstart = &v1alpha1.NameRef{Name: osBuildName}
		}
	}
	return kickstartConfigMap, osConfigTemplate, nil
}

func (o *OSBuildCreator) setKickstartConfigMapOwner(ctx context.Context, kickstartConfigMap *corev1.ConfigMap, osBuild *v1alpha1.OSBuild, scheme *runtime.Scheme, configMapRepository configmap.Repository) error {
	oldConfigMap := kickstartConfigMap.DeepCopy()
	err := controllerutil.SetOwnerReference(osBuild, kickstartConfigMap, scheme)
	if err != nil {
		return err
	}
	return configMapRepository.Patch(ctx, oldConfigMap, kickstartConfigMap)
}

func (o *OSBuildCreator) createKickstartConfigMap(ctx context.Context, osBuildConfig *v1alpha1.OSBuildConfig, osConfigTemplate *v1alpha1.OSBuildConfigTemplate, configMapRepository configmap.Repository, name, namespace string) (*corev1.ConfigMap, error) {
	kickstart, err := o.getKickstart(ctx, osConfigTemplate, osBuildConfig, configMapRepository)
	if err != nil {
		return nil, err
	}

	if kickstart == nil {
		return nil, nil
	}

	cm, err := configMapRepository.Read(ctx, name, namespace)
	if err == nil {
		// CM has already been created, returning it
		return cm, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}

	cm = &corev1.ConfigMap{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"kickstart": *kickstart,
		},
	}

	err = configMapRepository.Create(ctx, cm)
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func (o *OSBuildCreator) getKickstart(ctx context.Context, osConfigTemplate *v1alpha1.OSBuildConfigTemplate, osBuildConfig *v1alpha1.OSBuildConfig, configMapRepository configmap.Repository) (*string, error) {
	if osConfigTemplate.Spec.Iso == nil || osConfigTemplate.Spec.Iso.Kickstart == nil {
		return nil, nil
	}
	if osConfigTemplate.Spec.Iso.Kickstart.Raw == nil && osConfigTemplate.Spec.Iso.Kickstart.ConfigMapName == nil {
		return nil, nil
	}

	var kickstartTemplate string
	if osConfigTemplate.Spec.Iso.Kickstart.Raw != nil {
		kickstartTemplate = *osConfigTemplate.Spec.Iso.Kickstart.Raw
	} else {
		cm, err := configMapRepository.Read(ctx, *osConfigTemplate.Spec.Iso.Kickstart.ConfigMapName, osBuildConfig.Namespace)
		if err != nil {
			return nil, err
		}
		var ok bool
		if kickstartTemplate, ok = cm.Data["kickstart"]; !ok {
			return nil, errors.NewNotFound(schema.GroupResource{Group: "configmap", Resource: "key"}, "kickstart")
		}
	}

	finalKickstart, err := templates.ProcessOSBuildConfigTemplate(kickstartTemplate, osConfigTemplate.Spec.Parameters, osBuildConfig.Spec.Template.Parameters)
	if err != nil {
		return nil, err
	}
	return &finalKickstart, nil
}
