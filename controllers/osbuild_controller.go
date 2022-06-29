//go:generate go run -mod=mod github.com/deepmap/oapi-codegen/cmd/oapi-codegen -package=composer -old-config-style -generate=types,client -o ../internal/composer/client.go  ../internal/composer/openapi.v2.yml

/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	openapi_types "github.com/deepmap/oapi-codegen/pkg/types"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"net/http"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	osbuildv1alpha1 "github.com/project-flotta/osbuild-operator/api/v1alpha1"
	"github.com/project-flotta/osbuild-operator/internal/composer"
	repositoryosbuild "github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
)

const (
	FailedToSendPostRequestMsg = "Failed to post a new composer build request"

	// OSBuildConditionTypes values
	ContainerBuildDone    = "containerBuildDone"
	FailedContainerBuild  = "failedContainerBuild"
	StartedContainerBuild = "startedContainerBuild"
	IsoBuildDone          = "isoBuildDone"
	FailedIsoBuild        = "failedIsoBuild"
	StartedIsoBuild       = "startedIsoBuild"

	EmptyComposeID = ""
)

// OSBuildReconciler reconciles a OSBuild object
type OSBuildReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OSBuildRepository repositoryosbuild.Repository
	ComposerClient    *composer.Client
}

//+kubebuilder:rbac:groups=osbuilder.project-flotta.io,resources=osbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=osbuilder.project-flotta.io,resources=osbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=osbuilder.project-flotta.io,resources=osbuilds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OSBuild object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *OSBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("osbuild", req.Name)

	logger.Info("******* Reconcile1 Read from repository")
	osBuild, err := r.OSBuildRepository.Read(ctx, req.Name, req.Namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, err
	}

	if osBuild.DeletionTimestamp != nil {
		// The OSBuild CRs that were created by that OSBuildConfig would be deleted
		// thanks to setting controller reference for each OSBuild CR
		return ctrl.Result{}, nil
	}

	logger.Info("******* Reconcile2 Check ContainerComposeId is empty")
	var composeStatus composer.ComposeStatusValue

	if osBuild.Status.ContainerComposeId != "" {
		logger.Info("******* Reconcile3 ContainerComposeId is NOT empty")
		composeStatus, err = r.updateContainerComposeIDStatus(ctx, logger, osBuild)
		if err != nil {
			logger.Info("******* Reconcile3 error ContainerComposeId is NOT empty")
			return ctrl.Result{Requeue: true, RequeueAfter: 10}, nil
		}
	} else {
		// build edge container
		logger.Info("******* Reconcile4 ContainerComposeId is empty, postComposeEdgeContainer")
		err = r.postComposeEdgeContainer(ctx, logger, osBuild)
		if err != nil {
			logger.Info("******* Reconcile4 error ContainerComposeId is empty, postComposeEdgeContainer")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if osBuild.Status.IsoComposeId != "" {
		logger.Info("******* Reconcile5 IsoComposeId is NOT empty")
		composeStatus, err = r.updateInstallerComposeIDStatus(ctx, logger, osBuild)
		if err != nil {
			logger.Info("******* Reconcile5  errprIsoComposeId is NOT empty")
			return ctrl.Result{Requeue: true}, nil
		}
	} else if composeStatus == composer.ComposeStatusValueSuccess && osBuild.Spec.Details.TargetImage.TargetImageType == "edge-installer" {
		logger.Info("******* Reconcile6 postComposeEdgeInstaller")
		// build edge-installer
		err = r.postComposeEdgeInstaller(ctx, logger, osBuild)
		if err != nil {
			logger.Info("******* Reconcile6 error")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *OSBuildReconciler) updateContainerComposeIDStatus(ctx context.Context, logger logr.Logger, osBuild *osbuildv1alpha1.OSBuild) (composer.ComposeStatusValue, error) {
	composeStatus, err := r.checkComposeIDStatus(ctx, logger, uuid.MustParse(osBuild.Status.ContainerComposeId))
	if err != nil {
		logger.Error(err, "*******  error")
		return "", nil
	}

	return r.updateOSBuildConditionStatus(ctx, logger, osBuild, composeStatus, ContainerBuildDone, FailedContainerBuild, StartedContainerBuild)
}

func (r *OSBuildReconciler) updateInstallerComposeIDStatus(ctx context.Context, logger logr.Logger, osBuild *osbuildv1alpha1.OSBuild) (composer.ComposeStatusValue, error) {
	composeStatus, err := r.checkComposeIDStatus(ctx, logger, openapi_types.UUID(uuid.MustParse(osBuild.Status.IsoComposeId)))
	if err != nil {
		logger.Error(err, "*******  error")
		return "", nil
	}

	return r.updateOSBuildConditionStatus(ctx, logger, osBuild, composeStatus, IsoBuildDone, FailedIsoBuild, StartedIsoBuild)
}

func (r *OSBuildReconciler) updateOSBuildConditionStatus(ctx context.Context, logger logr.Logger,
	osBuild *osbuildv1alpha1.OSBuild, composeStatus composer.ComposeStatusValue,
	buildDoneValue osbuildv1alpha1.OSBuildConditionType, buildFailedValue osbuildv1alpha1.OSBuildConditionType,
	buildStartedValue osbuildv1alpha1.OSBuildConditionType) (composer.ComposeStatusValue, error) {

	if composeStatus == composer.ComposeStatusValueSuccess {
		r.updateOSBuildStatus(ctx, logger, osBuild, "", buildDoneValue, EmptyComposeID, EmptyComposeID)
	}

	if composeStatus == composer.ComposeStatusValueFailure {
		r.updateOSBuildStatus(ctx, logger, osBuild, "", buildFailedValue, EmptyComposeID, EmptyComposeID)
	}

	if composeStatus == composer.ComposeStatusValuePending {
		r.updateOSBuildStatus(ctx, logger, osBuild, "", buildStartedValue, EmptyComposeID, EmptyComposeID)
	}
	return composeStatus, nil
}

func (r *OSBuildReconciler) postComposeEdgeContainer(ctx context.Context, logger logr.Logger, osBuild *osbuildv1alpha1.OSBuild) error {
	logger.Info("******* postEdgeContainer1 postComposeEdgeContainer *****************")
	customizations, err := r.createCustomizations(*osBuild.Spec.Details.Customizations)
	if err != nil {
		return err
	}

	imageRequest, err := r.createImageRequest(osBuild.Spec.Details.TargetImage, "edge-container")
	if err != nil {
		return err
	}

	body := composer.PostComposeJSONRequestBody{
		Customizations: &customizations,
		Distribution:   osBuild.Spec.Details.Distribution,
		ImageRequest:   &imageRequest,
	}

	// post compos:
	response, err := r.ComposerClient.PostCompose(ctx, body)
	if err != nil {
		logger.Error(err, "******* postEdgeContainer2 error")
		r.updateOSBuildStatus(ctx, logger, osBuild, FailedToSendPostRequestMsg, FailedContainerBuild, EmptyComposeID, EmptyComposeID)
	}

	composerResponse, err := composer.ParsePostComposeResponse(response)
	if err != nil || composerResponse.StatusCode() != http.StatusCreated {
		logger.Info("******* postEdgeContainer3 status code", "composerResponse.StatusCode() ", composerResponse.StatusCode())
		logger.Error(err, "******* postEdgeContainer4 error ******** ")
		return err
	}

	logger.Info("******* postEdgeContainer5 ContainerComposeId FInISHED", "ContainerComposeId ", composerResponse.JSON201.Id.String())

	r.updateOSBuildStatus(ctx, logger, osBuild, "", StartedContainerBuild, composerResponse.JSON201.Id.String(), EmptyComposeID)
	return nil
}

func (r *OSBuildReconciler) updateOSBuildStatus(ctx context.Context, logger logr.Logger, osBuild *osbuildv1alpha1.OSBuild,
	msg string, typeCondition osbuildv1alpha1.OSBuildConditionType, containerComposeId string, isoComposeId string) {
	patch := client.MergeFrom(osBuild.DeepCopy())

	if containerComposeId != "" {
		osBuild.Status.ContainerComposeId = containerComposeId
	}

	if isoComposeId != "" {
		osBuild.Status.IsoComposeId = isoComposeId
	}

	if osBuild.Status.Conditions == nil {
		osBuild.Status.Conditions = []osbuildv1alpha1.OSBuildCondition{}
	}

	osBuild.Status.Conditions = append(osBuild.Status.Conditions, osbuildv1alpha1.OSBuildCondition{
		Type:    typeCondition,
		Message: &msg,
	})

	errPatch := r.OSBuildRepository.PatchStatus(ctx, osBuild, &patch)
	if errPatch != nil {
		logger.Error(errPatch, "Failed to patch OSBuild status")
	}
}

func (r *OSBuildReconciler) postComposeEdgeInstaller(ctx context.Context, logger logr.Logger, osBuild *osbuildv1alpha1.OSBuild) error {
	logger.Info("******* postEdgeInstaller1 Create Customizations")
	customizations, err := r.createCustomizations(*osBuild.Spec.Details.Customizations)
	if err != nil {
		logger.Info("******* postEdgeInstaller2 Failed Create Customizations")
		return err
	}

	logger.Info("******* postEdgeInstaller3 Create Imagerequest")
	imageRequest, err := r.createImageRequest(osBuild.Spec.Details.TargetImage, "edge-installer")
	if err != nil {
		logger.Info("******* postEdgeInstaller4 Failed Create Imagerequest")
		return err
	}

	body := composer.PostComposeJSONRequestBody{
		Customizations: &customizations,
		Distribution:   osBuild.Spec.Details.Distribution,
		ImageRequest:   &imageRequest,
	}
	logger.Info("******* postEdgeInstaller5 body", " BODY  ", body)

	// post compos:
	response, err := r.ComposerClient.PostCompose(ctx, body)
	if err != nil {
		logger.Error(err, "******* postEdgeInstaller6 error")
		r.updateOSBuildStatus(ctx, logger, osBuild, "", FailedIsoBuild, EmptyComposeID, EmptyComposeID)
		return err
	}

	logger.Info("******* postEdgeInstaller8 parse response", "response ", response)
	composerResponse, err := composer.ParsePostComposeResponse(response)
	logger.Info("******* postEdgeInstaller9 status code", "composerResponse.StatusCode() ", composerResponse.StatusCode())
	if err != nil || composerResponse.StatusCode() != http.StatusCreated {
		logger.Error(err, "******* postEdgeInstaller10 error")
		return err
	}

	logger.Info("******* postEdgeInstaller11 ContainerComposeId", "ContainerComposeId ", composerResponse.JSON201.Id.String())
	r.updateOSBuildStatus(ctx, logger, osBuild, "", StartedIsoBuild, EmptyComposeID, composerResponse.JSON201.Id.String())

	logger.Info("******* postEdgeInstaller13 FINISHED")
	return nil
}

func (r *OSBuildReconciler) checkComposeIDStatus(ctx context.Context, logger logr.Logger, composeID openapi_types.UUID) (composer.ComposeStatusValue, error) {
	response, err := r.ComposerClient.GetComposeStatus(ctx, composeID.String())
	if err != nil {
		logger.Error(err, "******* GetComposeStatus error")
		return "", err
	}
	composerResponse, err := composer.ParseGetComposeStatusResponse(response)
	if err != nil {
		logger.Error(err, "******* error parsing response")
		return "", err
	}
	if composerResponse.JSON200 != nil {
		return composerResponse.JSON200.Status, nil
	}
	return "", fmt.Errorf("something went wrong with requesting the composeID %v", composerResponse.StatusCode())
}

func (r *OSBuildReconciler) createImageRequest(targetImage osbuildv1alpha1.TargetImage, targetImageType osbuildv1alpha1.TargetImageType) (composer.ImageRequest, error) {
	uploadOptions := composer.UploadOptions(composer.AWSS3UploadOptions{Region: ""})

	imageRequest := composer.ImageRequest{
		Architecture:  string(targetImage.Architecture),
		ImageType:     composer.ImageTypes(targetImageType),
		Ostree:        (*composer.OSTree)(targetImage.OSTree.DeepCopy()),
		UploadOptions: &uploadOptions,
	}
	return imageRequest, nil
}

func (r *OSBuildReconciler) createCustomizations(osbuildCustomizations osbuildv1alpha1.Customizations) (composer.Customizations, error) {
	var users []composer.User
	for _, cstmzUser := range osbuildCustomizations.Users {
		user := (*composer.User)(cstmzUser.DeepCopy())
		users = append(users, *user)
	}

	composerCustomizations := composer.Customizations{
		Packages: &osbuildCustomizations.Packages,
		Users:    &users,
	}
	return composerCustomizations, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OSBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&osbuildv1alpha1.OSBuild{}).
		Complete(r)
}
