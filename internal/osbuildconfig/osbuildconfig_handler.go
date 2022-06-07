package osbuildconfig

import (
	"github.com/project-flotta/osbuild-operator/internal/manifests"
	repositoryosbuild "github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
	repositoryosbuildconfig "github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfig"
	"github.com/project-flotta/osbuild-operator/internal/repository/secret"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type OSBuildConfigHandler struct {
	OSBuildConfigRepository repositoryosbuildconfig.Repository
	SecretRepository        secret.Repository
	OSBuildRepository       repositoryosbuild.Repository
	Scheme                  *runtime.Scheme
	OSBuildCRCreator        manifests.OSBuildCRCreatorInterface
}

func NewOSBuildConfigHandler(osBuildConfigRepository repositoryosbuildconfig.Repository, osBuildRepository repositoryosbuild.Repository, secretRepository secret.Repository, scheme *runtime.Scheme, osBuildCRCreator manifests.OSBuildCRCreatorInterface) *OSBuildConfigHandler {
	return &OSBuildConfigHandler{
		OSBuildConfigRepository: osBuildConfigRepository,
		OSBuildRepository:       osBuildRepository,
		SecretRepository:        secretRepository,
		Scheme:                  scheme,
		OSBuildCRCreator:        osBuildCRCreator,
	}
}
func (o *OSBuildConfigHandler) OSBuildConfigWebhookTriggers(w http.ResponseWriter, r *http.Request, osbuildconfigNamespaceName string, osbuildconfigCrName string, secret string) {
	logger := log.FromContext(r.Context())
	logger.Info("OSBuildConfigWebhookTriggerHandler new request")

	// TODO - understand how to get params and use the responses
	osBuildConfig, err := o.OSBuildConfigRepository.Read(r.Context(), osbuildconfigCrName, osbuildconfigNamespaceName)
	if err != nil {
		if errors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		logger.Error(err, "can't read OSBuildConfigCR")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if osBuildConfig.Spec.Triggers.WebHook == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	secretName := osBuildConfig.Spec.Triggers.WebHook.SecretReference.Name
	secretVal := secret
	webhookSecret, err := o.SecretRepository.Read(r.Context(), secretName, osbuildconfigNamespaceName)
	if err != nil {
		if errors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		logger.Error(err, "can't read secret")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if string(webhookSecret.Data["WebHookSecretKey"]) != secretVal {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = o.OSBuildCRCreator.CreateNewOSBuildCR(r.Context(), osBuildConfig, logger, o.OSBuildConfigRepository, o.OSBuildRepository, o.Scheme)
	if err != nil {
		logger.Error(err, "can't create new OSBuild CR")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
