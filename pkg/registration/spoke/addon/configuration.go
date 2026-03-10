package addon

import (
	"context"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"

	addonapiv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

const (
	defaultAddOnInstallationNamespace = "open-cluster-management-agent-addon"
)

// registrationConfig contains necessary information for addon registration
// TODO: Refactor the code here once the registration configuration is available in spec of ManagedClusterAddOn
type registrationConfig struct {
	addOnName    string
	registration addonapiv1beta1.RegistrationConfig

	// secretName is the name of secret containing client certificate. If the SignerName is "kubernetes.io/kube-apiserver-client",
	// the secret name will be "{addon name}-hub-kubeconfig". Otherwise, the secret name will be "{addon name}-{signer name}-client-cert".
	secretName string
	hash       string
	stopFunc   context.CancelFunc

	addonInstallOption
}

type addonInstallOption struct {
	InstallationNamespace             string `json:"installationNamespace"`
	AgentRunningOutsideManagedCluster bool   `json:"agentRunningOutsideManagedCluster"`
}

// getKubeClientDriver extracts the kubeClient driver from registrations in v1beta1
// It returns the driver if there's a KubeClient type registration, otherwise empty string
func getKubeClientDriver(registrations []addonapiv1beta1.RegistrationConfig) string {
	for _, reg := range registrations {
		if reg.Type == addonapiv1beta1.RegistrationType("kubeClient") && reg.KubeClient != nil {
			return reg.KubeClient.Driver
		}
	}
	return ""
}

// getSubject extracts the subject from v1beta1 RegistrationConfig based on its type
func (c *registrationConfig) getSubject() (user string, groups []string, orgUnits []string) {
	switch c.registration.Type {
	case addonapiv1beta1.RegistrationType("kubeClient"):
		if c.registration.KubeClient != nil {
			return c.registration.KubeClient.Subject.User,
				c.registration.KubeClient.Subject.Groups,
				nil // KubeClient doesn't have OrganizationUnits
		}
	case addonapiv1beta1.RegistrationType("customSigner"):
		if c.registration.CustomSigner != nil {
			return c.registration.CustomSigner.Subject.User,
				c.registration.CustomSigner.Subject.Groups,
				c.registration.CustomSigner.Subject.OrganizationUnits
		}
	}
	return "", nil, nil
}

// getSignerName returns the signer name from v1beta1 RegistrationConfig
func (c *registrationConfig) getSignerName() string {
	if c.registration.Type == addonapiv1beta1.RegistrationType("kubeClient") {
		return certificatesv1.KubeAPIServerClientSignerName
	}
	if c.registration.Type == addonapiv1beta1.RegistrationType("customSigner") && c.registration.CustomSigner != nil {
		return c.registration.CustomSigner.SignerName
	}
	return ""
}

func (c *registrationConfig) x509Subject(clusterName, agentName string) *pkix.Name {
	user, groups, orgUnits := c.getSubject()
	subject := &pkix.Name{
		CommonName:         user,
		Organization:       groups,
		OrganizationalUnit: orgUnits,
	}

	// set the default common name
	if len(subject.CommonName) == 0 {
		subject.CommonName = defaultCommonName(clusterName, agentName, c.addOnName)
	}

	// set the default organization if signer is KubeAPIServerClientSignerName
	if c.getSignerName() == certificatesv1.KubeAPIServerClientSignerName && len(subject.Organization) == 0 {
		subject.Organization = []string{defaultOrganization(clusterName, c.addOnName)}
	}

	return subject
}

// getAddOnInstallationNamespace returns addon installation namespace from addon spec.
// It first checks the installation namespace in status then annotation (v1alpha1 compatibility), the addon default
// installation namespace open-cluster-management-agent-addon will be returned.
func getAddOnInstallationNamespace(addOn *addonapiv1beta1.ManagedClusterAddOn) string {
	installationNamespace := addOn.Status.Namespace
	if installationNamespace == "" {
		installationNamespace = addOn.Annotations[addonapiv1beta1.InstallNamespaceAnnotation]
	}
	if installationNamespace == "" {
		installationNamespace = defaultAddOnInstallationNamespace
	}

	return installationNamespace
}

// isAddonRunningOutsideManagedCluster returns whether the addon agent is running on the managed cluster
func isAddonRunningOutsideManagedCluster(addOn *addonapiv1beta1.ManagedClusterAddOn) bool {
	hostingCluster, ok := addOn.Annotations[addonapiv1beta1.HostingClusterNameAnnotationKey]
	if ok && len(hostingCluster) != 0 {
		return true
	}
	return false
}

// getRegistrationConfigs reads registrations and returns a map of registrationConfig whose
// key is the hash of the registrationConfig.
func getRegistrationConfigs(
	addOnName string,
	installOption addonInstallOption,
	registrations []addonapiv1beta1.RegistrationConfig,
	kubeClientDriver string,
) (map[string]registrationConfig, error) {
	configs := map[string]registrationConfig{}

	for _, registration := range registrations {
		config := registrationConfig{
			addOnName:          addOnName,
			addonInstallOption: installOption,
			registration:       registration,
		}

		// set the secret name of client certificate
		signerName := ""
		if registration.Type == addonapiv1beta1.RegistrationType("kubeClient") {
			signerName = certificatesv1.KubeAPIServerClientSignerName
			config.secretName = fmt.Sprintf("%s-hub-kubeconfig", addOnName)
		} else if registration.Type == addonapiv1beta1.RegistrationType("customSigner") && registration.CustomSigner != nil {
			signerName = registration.CustomSigner.SignerName
			config.secretName = fmt.Sprintf("%s-%s-client-cert", addOnName, strings.ReplaceAll(signerName, "/", "-"))
		}

		// hash registration configuration, install namespace and addOnAgentRunningOutsideManagedCluster. Use the hash
		// value as the key of map to make sure each registration configuration and addon installation option is unique
		hash, err := getConfigHash(
			registration,
			config.addonInstallOption,
			kubeClientDriver)
		if err != nil {
			return configs, err
		}
		config.hash = hash
		configs[config.hash] = config
	}

	return configs, nil
}

func getConfigHash(registration addonapiv1beta1.RegistrationConfig, installOption addonInstallOption, kubeClientDriver string) (string, error) {
	// Create a canonical config for hashing, excluding status fields set by the agent.
	// Driver is always excluded (set by agent as status, not configuration)
	// Subject is excluded only for token-based authentication (set by token driver)
	// Subject is included for CSR-based and custom signer authentication (part of the configuration)
	canonicalConfig := addonapiv1beta1.RegistrationConfig{
		Type: registration.Type,
	}

	// For KubeClient type registrations, check if driver is token
	// For custom signers, always include subject
	if registration.Type == addonapiv1beta1.RegistrationType("kubeClient") {
		if kubeClientDriver != "token" && registration.KubeClient != nil {
			// Include subject for non-token auth
			canonicalConfig.KubeClient = &addonapiv1beta1.KubeClientConfig{
				Subject: registration.KubeClient.Subject,
			}
		}
	} else if registration.Type == addonapiv1beta1.RegistrationType("customSigner") {
		// Always include custom signer config
		canonicalConfig.CustomSigner = registration.CustomSigner
	}

	data, err := json.Marshal(canonicalConfig)
	if err != nil {
		return "", err
	}

	installOptionData, err := json.Marshal(installOption)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	h.Write(data)
	h.Write(installOptionData)

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func defaultCommonName(clusterName, agentName, addonName string) string {
	return fmt.Sprintf("%s:agent:%s", defaultOrganization(clusterName, addonName), agentName)
}

func defaultOrganization(clusterName, addonName string) string {
	return fmt.Sprintf("system:open-cluster-management:cluster:%s:addon:%s", clusterName, addonName)
}
