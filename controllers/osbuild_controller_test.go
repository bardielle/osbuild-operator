package controllers_test

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"github.com/project-flotta/osbuild-operator/internal/composer"
	"github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"

	osbuildv1alpha1 "github.com/project-flotta/osbuild-operator/api/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	. "github.com/onsi/gomega"

	"github.com/project-flotta/osbuild-operator/internal/conf"

	"github.com/project-flotta/osbuild-operator/controllers"
)

var _ = Describe("OSBuildEnvConfig Controller", func() {
	const (
		instanceNamespace = "osbuild"
		instanceName      = "osbuild_test"
	)
	var (
		mockCtrl          *gomock.Controller
		scheme            *runtime.Scheme
		osBuildRepository *osbuild.MockRepository
		//composerClient    *composer.Client
		composerClient  *composer.MockClientWithResponsesInterface
		reconciler      *controllers.OSBuildReconciler
		requestContext  context.Context
		osbuildInstance osbuildv1alpha1.OSBuild
		request         = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name: instanceName,
			},
		}

		composerResponseCreated composer.PostComposeResponse
		composerResponseFailed  composer.PostComposeResponse

		resultShortRequeue = ctrl.Result{Requeue: true, RequeueAfter: controllers.RequeueForShortDuration}
		resultLongRequeue  = ctrl.Result{Requeue: true, RequeueAfter: controllers.RequeueForLongDuration}
		resultDone         = ctrl.Result{}

		errNotFound  error
		errFailed    error
		packages     = []string{"pkg1", "pkg2"}
		sshPublicKey = "publicKey"

		usr1 = osbuildv1alpha1.User{
			Groups: &[]string{"group1", "group2"},
			Key:    &sshPublicKey,
			Name:   "usr1",
		}
		usr2 = osbuildv1alpha1.User{
			Groups: &[]string{"group3", "group4"},
			Key:    &sshPublicKey,
			Name:   "usr2",
		}
		disabledServices = []string{"s1", "s2"}
		enabledServices  = []string{"s3", "s4"}
	)
	BeforeEach(func() {
		err := conf.Load()
		Expect(err).To(BeNil())

		mockCtrl = gomock.NewController(GinkgoT())
		osBuildRepository = osbuild.NewMockRepository(mockCtrl)
		composerClient = composer.NewMockClientWithResponsesInterface(mockCtrl)

		scheme = runtime.NewScheme()
		err = clientgoscheme.AddToScheme(scheme)
		Expect(err).To(BeNil())
		err = osbuildv1alpha1.AddToScheme(scheme)

		reconciler = &controllers.OSBuildReconciler{
			Scheme:            scheme,
			OSBuildRepository: osBuildRepository,
			ComposerClient:    composerClient,
		}

		requestContext = context.TODO()

		errNotFound = errors.NewNotFound(schema.GroupResource{}, "Requested resource was not found")
		errFailed = errors.NewInternalError(fmt.Errorf("Server encounter and error"))

		osbuildInstance = osbuildv1alpha1.OSBuild{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: instanceNamespace,
			},
			Spec: osbuildv1alpha1.OSBuildSpec{
				Details: osbuildv1alpha1.BuildDetails{
					Distribution: "rhel-86",
					Customizations: &osbuildv1alpha1.Customizations{
						Packages: packages,
						Users:    []osbuildv1alpha1.User{usr1, usr2},
						Services: &osbuildv1alpha1.Services{
							Disabled: disabledServices,
							Enabled:  enabledServices,
						},
					},
					TargetImage: osbuildv1alpha1.TargetImage{
						Architecture:    "x86_64",
						TargetImageType: "edge-container",
						OSTree:          nil,
					},
				},
				Kickstart:   nil,
				TriggeredBy: "UpdateCR",
			},
			Status: osbuildv1alpha1.OSBuildStatus{},
		}

		composerResponseCreated = composer.PostComposeResponse{
			HTTPResponse: &http.Response{
				StatusCode: http.StatusCreated,
			},
		}
		composerResponseFailed = composer.PostComposeResponse{
			Body: nil,
			HTTPResponse: &http.Response{
				StatusCode: http.StatusBadRequest,
			},
		}

	})

	AfterEach(func() {
		mockCtrl.Finish()
		osbuildInstance.DeletionTimestamp = nil
		osbuildInstance.Status.Conditions = nil
		osbuildInstance.Status.ContainerComposeId = controllers.EmptyComposeID
	})

	Context("Failure to get OSBuild instance", func() {
		It("Should return Done when the instance is not found", func() {
			// given
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(nil, errNotFound)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(BeNil())
			Expect(result).To(Equal(resultDone))

		})

		It("Should return error when failed to get the instance", func() {
			// given
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(nil, errFailed)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(Equal(errFailed))
			Expect(result).To(Equal(resultShortRequeue))
		})
	})

	Context("Handle deletion", func() {
		It("Should return Done if removed the finalizer successfully", func() {
			// given
			osbuildInstance.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(osbuildInstance, nil)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(BeNil())
			Expect(result).To(Equal(resultDone))
		})
	})

	Context("Create edge-container request ", func() {
		It("and fail on postCompose with error", func() {
			// given
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(nil, errFailed)
			osBuildRepository.EXPECT().PatchStatus(requestContext, gomock.Any(), gomock.Any()).Return(nil)
			composerClient.EXPECT().PostComposeWithResponse(requestContext, gomock.Any()).Return(nil, errFailed)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(BeNil())
			Expect(result).To(Equal(resultLongRequeue))
		})
		It("and on postCompose", func() {
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(nil, errFailed)
			osBuildRepository.EXPECT().PatchStatus(requestContext, gomock.Any(), gomock.Any()).Return(nil)
			composerClient.EXPECT().PostComposeWithResponse(requestContext, gomock.Any()).Return(composerResponseFailed, nil)

			// given
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(osbuildInstance, nil)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(BeNil())
			Expect(result).To(Equal(resultDone))
		})
	})

	Context("Update edge-container job status", func() {
		It("failed postCompose request", func() {
			osbuildInstance.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			// given
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(osbuildInstance, nil)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(BeNil())
			Expect(result).To(Equal(resultDone))
		})
		It("send postCompose request", func() {
			osbuildInstance.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			// given
			osBuildRepository.EXPECT().Read(requestContext, instanceName, instanceNamespace).Return(osbuildInstance, nil)
			// when
			result, err := reconciler.Reconcile(requestContext, request)
			// then
			Expect(err).To(BeNil())
			Expect(result).To(Equal(resultDone))
		})
	})
})
