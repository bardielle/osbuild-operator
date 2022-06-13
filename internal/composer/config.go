package composer

const (
	osBuildOperatorFinalizer = "osbuilder.project-flotta.io/osBuildOperatorFinalizer"

	CACertsSecretName          = "ca-certificate"
	caCertsSecretPrivateKeyKey = "ca-key.pem"
	caCertsSecretPublicCertKey = "ca-crt.pem"

	composerCertsSecretName          = "composer-certificate"
	composerCertsSecretPrivateKeyKey = "composer-key.pem"
	composerCertsSecretPublicCertKey = "composer-crt.pem"

	composerConfigMapName      = "osbuild-composer-config"
	composerConfigMapKey       = "osbuild-composer.toml"
	composerConfigTemplateFile = "osbuild-composer.toml"

	composerProxyConfigMapName      = "osbuild-composer-proxy-config"
	composerProxyConfigMapKey       = "envoy.yaml"
	composerProxyConfigTemplateFile = "composer-proxy-config.yaml"

	composerDeploymentName         = "composer"
	composerDeploymentTemplateFile = "composer-deployment.yaml"

	ComposerAPIServiceName = "osbuild-composer"
	ComposerAPIPortName    = "composer-api"
	WorkerAPIServiceName   = "osbuild-worker"
	WorkerAPIPortName      = "worker-api"

	pgSSLModeDefault = "prefer"

	workerSSHKeysSecretName    = "osbuild-worker-ssh"
	workerSSHKeysPrivateKeyKey = "ssh-privatekey"
	workerSSHKeysPublicKeyKey  = "ssh-publickey"

	workerVMUsername     = "cloud-user"
	workerVMTemplateFile = "worker-vm.yaml"
)
