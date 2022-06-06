package manifests

import (
	"context"
	_ "github.com/golang/mock/mockgen/model"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/go-logr/logr"
	osbuilderprojectflottaiov1alpha1 "github.com/project-flotta/osbuild-operator/api/v1alpha1"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate mockgen -package=manifests -source=osbuild.go -destination=mock_osbuildcrcreator.go
type OSBuildCRCreatorInterface interface {
	CreateNewOSBuildCR(ctx context.Context, osBuildConfig *osbuilderprojectflottaiov1alpha1.OSBuildConfig,
		logger logr.Logger, osBuildConfigRepository osbuildconfig.Repository, osBuildRepository osbuild.Repository,
		scheme *runtime.Scheme) error
}
type OSBuildCRCreator struct{}

func NewOSBuildCRCreator() OSBuildCRCreatorInterface {
	return &OSBuildCRCreator{}
}

func (o *OSBuildCRCreator) CreateNewOSBuildCR(ctx context.Context, osBuildConfig *osbuilderprojectflottaiov1alpha1.OSBuildConfig,
	logger logr.Logger, osBuildConfigRepository osbuildconfig.Repository, osBuildRepository osbuild.Repository,
	scheme *runtime.Scheme) error {

	var osBuildNewVersion int
	if osBuildConfig.Status.LastVersion != nil {
		osBuildNewVersion = *osBuildConfig.Status.LastVersion + 1
	} else {
		osBuildNewVersion = 1
	}

	osBuildConfigSpecDetails := osBuildConfig.Spec.Details.DeepCopy()
	osBuild := &osbuilderprojectflottaiov1alpha1.OSBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name: osBuildConfig.Name + "-" + string(rune(osBuildNewVersion)),
		},
		Spec: osbuilderprojectflottaiov1alpha1.OSBuildSpec{
			Details:     *osBuildConfigSpecDetails,
			TriggeredBy: "UpdateCR",
		},
	}

	// Set the owner of the osBuild CR to be osBuildConfig in order to manage lifecycle of the osBuild CR.
	// Especially in deletion of osBuildConfig CR
	err := controllerutil.SetControllerReference(osBuildConfig, osBuild, scheme)
	if err != nil {
		logger.Error(err, "cannot create osBuild")
		return err
	}

	patch := client.MergeFrom(osBuildConfig.DeepCopy())
	osBuildConfig.Status.LastVersion = &osBuildNewVersion
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

	logger.Info("A new OSBuild CR was created", osBuild.Name)

	return nil
}
