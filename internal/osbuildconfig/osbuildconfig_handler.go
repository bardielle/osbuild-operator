package osbuildconfig

import (
	"fmt"
	"github.com/project-flotta/osbuild-operator/internal/repository/configmap"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfigtemplate"
	"net/http"

	loggerutil "github.com/project-flotta/osbuild-operator/internal/logger"
	"github.com/project-flotta/osbuild-operator/internal/manifests"
	repositoryosbuild "github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
	repositoryosbuildconfig "github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfig"
	"github.com/project-flotta/osbuild-operator/internal/repository/secret"
	"github.com/project-flotta/osbuild-operator/restapi"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

type OSBuildConfigHandler struct {
	OSBuildConfigRepository         repositoryosbuildconfig.Repository
	SecretRepository                secret.Repository
	OSBuildRepository               repositoryosbuild.Repository
	Scheme                          *runtime.Scheme
	OSBuildCRCreator                manifests.OSBuildCRCreator
	OSBuildConfigTemplateRepository osbuildconfigtemplate.Repository
	ConfigMapRepository             configmap.Repository
}

func NewOSBuildConfigHandler(osBuildConfigRepository repositoryosbuildconfig.Repository,
	osBuildRepository repositoryosbuild.Repository, secretRepository secret.Repository, scheme *runtime.Scheme,
	osBuildCRCreator manifests.OSBuildCRCreator, osBuildConfigTemplateRepository osbuildconfigtemplate.Repository,
	configMapRepository configmap.Repository) *OSBuildConfigHandler {
	return &OSBuildConfigHandler{
		OSBuildConfigRepository:         osBuildConfigRepository,
		OSBuildRepository:               osBuildRepository,
		SecretRepository:                secretRepository,
		Scheme:                          scheme,
		OSBuildCRCreator:                osBuildCRCreator,
		ConfigMapRepository:             configMapRepository,
		OSBuildConfigTemplateRepository: osBuildConfigTemplateRepository,
	}
}
func (o *OSBuildConfigHandler) TriggerBuild(w http.ResponseWriter, r *http.Request, namespace string, name string, params restapi.TriggerBuildParams) {
	err, logger := loggerutil.Logger()
	if err != nil {
		return
	}

	logger.Info("New OSBuild trigger was sent ", "OSBuildConfig", name, "namespace", namespace)

	osBuildConfig, err := o.OSBuildConfigRepository.Read(r.Context(), name, namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error("resource OSBuildConfig not found")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		logger.Error(err, fmt.Sprintf("cannot retrieve OSBuildConfig %s", name))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if osBuildConfig.Spec.Triggers.WebHook == nil {
		logger.Error("resource OSBuildConfig doesn't support triggers by webhook")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	secretName := osBuildConfig.Spec.Triggers.WebHook.SecretReference.Name
	secretVal := params.Secret
	webhookSecret, err := o.SecretRepository.Read(r.Context(), secretName, namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error("secret not found", "secret", secretName)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		logger.Error(err, "cannot read secret")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if string(webhookSecret.Data["WebHookSecretKey"]) != secretVal {
		logger.Error("secret value is forbidden")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = o.OSBuildCRCreator.Create(r.Context(), osBuildConfig, o.OSBuildConfigRepository, o.OSBuildRepository,
		o.OSBuildConfigTemplateRepository, o.ConfigMapRepository, o.Scheme)
	if err != nil {
		logger.Error(err, "cannot create new OSBuild CR")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	logger.Info("new CR of OSBuild was created")
	w.WriteHeader(http.StatusOK)
}
