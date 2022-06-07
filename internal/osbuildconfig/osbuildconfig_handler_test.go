package osbuildconfig

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/project-flotta/osbuild-operator/api/v1alpha1"
	"github.com/project-flotta/osbuild-operator/internal/manifests"
	repositoryosbuild "github.com/project-flotta/osbuild-operator/internal/repository/osbuild"
	repositoryosbuildconfig "github.com/project-flotta/osbuild-operator/internal/repository/osbuildconfig"
	repositorysecret "github.com/project-flotta/osbuild-operator/internal/repository/secret"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	Namespace         = "test_namespace"
	SecretName        = "test_secret"
	OSBuildConfigName = "test_osbuildconfig"
)

func TestOsbuildConfigAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "test")
}

var _ = Describe("OSBuildConfig rest API", func() {
	var (
		mockCtrl       *gomock.Controller
		osbuildConfig  v1alpha1.OSBuildConfig
		triggerWebhook *buildv1.WebHookTrigger
		secret         corev1.Secret
		secretVal      = "c2VjcmV0dmFsdWUx"
		secretData     = map[string][]byte{
			"WebHookSecretKey": []byte(secretVal),
		}

		osBuildConfigRepository *repositoryosbuildconfig.MockRepository
		osBuildRepository       *repositoryosbuild.MockRepository
		secretRepository        *repositorysecret.MockRepository
		osBuildCRCreator        *manifests.MockOSBuildCRCreatorInterface
		responseWriter          *httptest.ResponseRecorder
		req                     *http.Request
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		osBuildRepository = repositoryosbuild.NewMockRepository(mockCtrl)
		osBuildConfigRepository = repositoryosbuildconfig.NewMockRepository(mockCtrl)
		osBuildCRCreator = manifests.NewMockOSBuildCRCreatorInterface(mockCtrl)
		secretRepository = repositorysecret.NewMockRepository(mockCtrl)

		secret = corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Namespace: Namespace,
				Name:      SecretName,
			},
			Data: secretData,
		}
		secretReference := &buildv1.SecretLocalReference{
			Name: SecretName,
		}
		triggerWebhook = &buildv1.WebHookTrigger{
			SecretReference: secretReference,
		}

		triggerNewBuildUponAnyChange := true
		osbuildConfig = v1alpha1.OSBuildConfig{
			ObjectMeta: v1.ObjectMeta{
				Name: OSBuildConfigName,
			},
			Spec: v1alpha1.OSBuildConfigSpec{
				Details: v1alpha1.BuildDetails{
					Distribution:   "rhel-86",
					Customizations: nil,
					TargetImage: v1alpha1.TargetImage{
						Architecture:    "x86_64",
						TargetImageType: "edge-container",
						OSTree:          nil,
					},
				},
				Triggers: v1alpha1.BuildTriggers{
					ConfigChange: &triggerNewBuildUponAnyChange,
					WebHook:      triggerWebhook,
				},
			},
			Status: v1alpha1.OSBuildConfigStatus{},
		}

		responseWriter = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "test_request", nil)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("trigger a build", func() {
		It("and succeed", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(&osbuildConfig, nil)
			secretRepository.EXPECT().Read(req.Context(), SecretName, Namespace).Return(&secret, nil)

			osBuildCRCreator.EXPECT().CreateNewOSBuildCR(req.Context(), &osbuildConfig, gomock.Any(), osBuildConfigRepository, osBuildRepository, nil).Return(nil)
			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusOK))

		})

		It("with not found response, because osbuildConfig doesn't exist", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			returnErr := errors.NewNotFound(schema.GroupResource{Group: "", Resource: "notfound"}, "notfound")
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(nil, returnErr)

			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusNotFound))
		})

		It("with internalServerError response, because osbuildConfigRepository failed", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			returnErr := errors.NewBadRequest("test")
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(nil, returnErr)

			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusInternalServerError))
		})

		It("with bad request response, because osbuildConfig hasn't webhook", func() {
			// given

			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(&osbuildConfig, nil)
			osbuildConfig.Spec.Triggers.WebHook = nil

			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("with not found response, because the secret doesn't exist", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(&osbuildConfig, nil)
			returnErr := errors.NewNotFound(schema.GroupResource{Group: "", Resource: "notfound"}, "notfound")
			secretRepository.EXPECT().Read(req.Context(), SecretName, Namespace).Return(nil, returnErr)

			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusNotFound))
		})

		It("with internalServerError response, because getting the secret failed", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(&osbuildConfig, nil)
			returnErr := errors.NewBadRequest("test")
			secretRepository.EXPECT().Read(req.Context(), SecretName, Namespace).Return(nil, returnErr)

			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusInternalServerError))
		})

		It("with forbidden response, because the webhook secret value different from the input param", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(&osbuildConfig, nil)
			secretRepository.EXPECT().Read(req.Context(), SecretName, Namespace).Return(&secret, nil)
			invalidSecretVal := "123"

			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, invalidSecretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusForbidden))

		})

		It("with internalServerError response, because the creation of the osbuild CR failed", func() {
			// given
			osbuildConfigHandler := NewOSBuildConfigHandler(osBuildConfigRepository, osBuildRepository, secretRepository, nil, osBuildCRCreator)
			osBuildConfigRepository.EXPECT().Read(req.Context(), OSBuildConfigName, Namespace).Return(&osbuildConfig, nil)
			secretRepository.EXPECT().Read(req.Context(), SecretName, Namespace).Return(&secret, nil)
			returnErr := errors.NewBadRequest("test")
			osBuildCRCreator.EXPECT().CreateNewOSBuildCR(gomock.Any(), &osbuildConfig, gomock.Any(), osBuildConfigRepository, osBuildRepository, nil).Return(returnErr)
			// when
			osbuildConfigHandler.OSBuildConfigWebhookTriggers(responseWriter, req, Namespace, OSBuildConfigName, secretVal)

			// then
			Expect(responseWriter.Result().StatusCode).To(Equal(http.StatusInternalServerError))

		})
	})

})
