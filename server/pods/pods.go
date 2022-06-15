package pods

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spiffe/spire/pkg/agent/common/cgroups"
	"github.com/spiffe/spire/pkg/common/pemutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
)

const (
	defaultMaxPollAttempts     = 60
	defaultPollRetryInterval   = time.Millisecond * 500
	defaultSecureKubeletPort   = 10250
	defaultKubeletCAPath       = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	defaultTokenPath           = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint: gosec // false positive
	defaultNodeNameEnv         = "MY_NODE_NAME"
	defaultReloadInterval      = time.Minute
	defaultContainerMountPoint = "CONTAINER_SANDBOX_MOUNT_POINT"
)

type containerLookup int

const (
	containerInPod = iota
	containerNotInPod
)

// k8sConfig holds the configuration distilled from HCL
type k8sConfig struct {
	Secure                  bool
	Port                    int
	MaxPollAttempts         int
	PollRetryInterval       time.Duration
	SkipKubeletVerification bool
	TokenPath               string
	CertificatePath         string
	PrivateKeyPath          string
	KubeletCAPath           string
	NodeName                string
	ReloadInterval          time.Duration

	Client     *kubeletClient
	LastReload time.Time
}

func loadToken(path string) (string, error) {
	if path == "" {
		mountPoint := os.Getenv(defaultContainerMountPoint)
		path = mountPoint + defaultTokenPath
	}
	token, err := readFile(path)
	if err != nil {
		return "", status.Errorf(codes.InvalidArgument, "unable to load token: %v", err)
	}
	return strings.TrimSpace(string(token)), nil
}

func loadKubeletCA(path string) (*x509.CertPool, error) {
	if path == "" {
		mountPoint := os.Getenv(defaultContainerMountPoint)
		path = mountPoint + defaultKubeletCAPath
	}
	caPEM, err := readFile(path)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unable to load kubelet CA: %v", err)
	}
	certs, err := pemutil.ParseCertificates(caPEM)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unable to parse kubelet CA: %v", err)
	}

	return newCertPool(certs), nil
}

func readFile(path string) ([]byte, error) {
	fs := cgroups.OSFileSystem{}
	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func NewClient() (*Client, error) {
	mountPoint := os.Getenv(defaultContainerMountPoint)

	config := &k8sConfig{
		Port:                    defaultSecureKubeletPort,
		TokenPath:               mountPoint + defaultTokenPath,
		NodeName:                "winw1",
		SkipKubeletVerification: true,
	}
	// // The insecure client only needs to be loaded once.
	// if !config.Secure {
	// if config.Client == nil {
	// config.Client = &kubeletClient{
	// URL: url.URL{
	// Scheme: "http",
	// Host:   fmt.Sprintf("127.0.0.1:%d", config.Port),
	// },
	// }
	// }
	// return nil
	// }

	// Is the client still fresh?
	// if config.Client != nil && p.clock.Now().Sub(config.LastReload) < config.ReloadInterval {
	// return nil
	// }

	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipKubeletVerification, //nolint: gosec // intentionally configurable
	}

	// var rootCAs *x509.CertPool
	// if !config.SkipKubeletVerification {
	// rootCAs, err = p.loadKubeletCA(config.KubeletCAPath)
	// if err != nil {
	// return err
	// }
	// }
	rootCAs, err := loadKubeletCA(config.KubeletCAPath)
	if err != nil {
		return nil, err
	}

	// switch {
	// case config.SkipKubeletVerification:

	// // When contacting the kubelet over localhost, skip the hostname validation.
	// // Unfortunately Go does not make this straightforward. We disable
	// // verification but supply a VerifyPeerCertificate that will be called
	// // with the raw kubelet certs that we can verify directly.
	// case config.NodeName == "":
	// tlsConfig.InsecureSkipVerify = true
	// tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	// var certs []*x509.Certificate
	// for _, rawCert := range rawCerts {
	// cert, err := x509.ParseCertificate(rawCert)
	// if err != nil {
	// return err
	// }
	// certs = append(certs, cert)
	// }

	// // this is improbable.
	// if len(certs) == 0 {
	// return errors.New("no certs presented by kubelet")
	// }

	// _, err := certs[0].Verify(x509.VerifyOptions{
	// Roots:         rootCAs,
	// Intermediates: newCertPool(certs[1:]),
	// })
	// return err
	// }
	// default:

	// DIsable CA
	tlsConfig.RootCAs = rootCAs

	// }

	token, err := loadToken(config.TokenPath)
	if err != nil {
		return nil, err
	}

	host := config.NodeName
	if host == "" {
		host = "127.0.0.1"
	}

	config.Client = &kubeletClient{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		URL: url.URL{
			Scheme: "https",
			Host:   fmt.Sprintf("%s:%d", host, config.Port),
		},
		Token: token,
	}
	// config.LastReload = p.clock.Now()

	return &Client{
		c: config,
	}, nil
}

type Client struct {
	c *k8sConfig
}

func (c *Client) GetPodByContainer(containerID string) ([]string, error) {
	list, err := c.c.Client.GetPodList()
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		item := item
		log.Println("----------------------------------------------")
		log.Printf("%+v\n", item)
		log.Println("----------------------------------------------")
		// if item.UID != podUID {
		// continue
		// }

		status, lookup := lookUpContainerInPod(containerID, item.Status)
		switch lookup {
		case containerInPod:
			return getSelectorValuesFromPodInfo(&item, status), nil
		case containerNotInPod:
		}
	}
	return nil, errors.New("not found")
}

func getPodImageIdentifiers(containerStatusArray []corev1.ContainerStatus) map[string]bool {
	// Map is used purely to exclude duplicate selectors, value is unused.
	podImages := make(map[string]bool)
	// Note that for each pod image we generate *2* matching selectors.
	// This is to support matching against ImageID, which has a SHA
	// docker.io/envoyproxy/envoy-alpine@sha256:bf862e5f5eca0a73e7e538224578c5cf867ce2be91b5eaed22afc153c00363eb
	// as well as
	// docker.io/envoyproxy/envoy-alpine:v1.16.0, which does not,
	// while also maintaining backwards compatibility and allowing for dynamic workload registration (k8s operator)
	// when the SHA is not yet known (e.g. before the image pull is initiated at workload creation time)
	// More info here: https://github.com/spiffe/spire/issues/2026
	for _, status := range containerStatusArray {
		podImages[status.ImageID] = true
		podImages[status.Image] = true
	}
	return podImages
}

func getSelectorValuesFromPodInfo(pod *corev1.Pod, status *corev1.ContainerStatus) []string {
	podImageIdentifiers := getPodImageIdentifiers(pod.Status.ContainerStatuses)
	podInitImageIdentifiers := getPodImageIdentifiers(pod.Status.InitContainerStatuses)
	containerImageIdentifiers := getPodImageIdentifiers([]corev1.ContainerStatus{*status})

	selectorValues := []string{
		fmt.Sprintf("sa:%s", pod.Spec.ServiceAccountName),
		fmt.Sprintf("ns:%s", pod.Namespace),
		fmt.Sprintf("node-name:%s", pod.Spec.NodeName),
		fmt.Sprintf("pod-uid:%s", pod.UID),
		fmt.Sprintf("pod-name:%s", pod.Name),
		fmt.Sprintf("container-name:%s", status.Name),
		fmt.Sprintf("pod-image-count:%s", strconv.Itoa(len(pod.Status.ContainerStatuses))),
		fmt.Sprintf("pod-init-image-count:%s", strconv.Itoa(len(pod.Status.InitContainerStatuses))),
	}

	for containerImage := range containerImageIdentifiers {
		selectorValues = append(selectorValues, fmt.Sprintf("container-image:%s", containerImage))
	}
	for podImage := range podImageIdentifiers {
		selectorValues = append(selectorValues, fmt.Sprintf("pod-image:%s", podImage))
	}
	for podInitImage := range podInitImageIdentifiers {
		selectorValues = append(selectorValues, fmt.Sprintf("pod-init-image:%s", podInitImage))
	}

	for k, v := range pod.Labels {
		selectorValues = append(selectorValues, fmt.Sprintf("pod-label:%s:%s", k, v))
	}
	for _, ownerReference := range pod.OwnerReferences {
		selectorValues = append(selectorValues, fmt.Sprintf("pod-owner:%s:%s", ownerReference.Kind, ownerReference.Name))
		selectorValues = append(selectorValues, fmt.Sprintf("pod-owner-uid:%s:%s", ownerReference.Kind, ownerReference.UID))
	}

	return selectorValues
}

type kubeletClient struct {
	Transport *http.Transport
	URL       url.URL
	Token     string
}

func (c *kubeletClient) GetPodList() (*corev1.PodList, error) {
	url := c.URL
	url.Path = "/pods"
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unable to create request: %v", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	client := &http.Client{}
	if c.Transport != nil {
		client.Transport = c.Transport
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unable to perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.Internal, "unexpected status code on pods response: %d %s", resp.StatusCode, tryRead(resp.Body))
	}

	out := new(corev1.PodList)
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, status.Errorf(codes.Internal, "unable to decode kubelet response: %v", err)
	}

	return out, nil
}

func tryRead(r io.Reader) string {
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func lookUpContainerInPod(containerID string, status corev1.PodStatus) (*corev1.ContainerStatus, containerLookup) {
	for _, status := range status.ContainerStatuses {
		// TODO: should we be keying off of the status or is the lack of a
		// container id sufficient to know the container is not ready?
		if status.ContainerID == "" {
			continue
		}

		containerURL, err := url.Parse(status.ContainerID)
		if err != nil {
			log.Printf("Malformed container id %q: %v", status.ContainerID, err)
			continue
		}

		if containerID == containerURL.Host {
			return &status, containerInPod
		}
	}

	for _, status := range status.InitContainerStatuses {
		// TODO: should we be keying off of the status or is the lack of a
		// container id sufficient to know the container is not ready?
		if status.ContainerID == "" {
			continue
		}

		containerURL, err := url.Parse(status.ContainerID)
		if err != nil {
			log.Printf("Malformed container id %q: %v", status.ContainerID, err)
			continue
		}

		if containerID == containerURL.Host {
			return &status, containerInPod
		}
	}

	return nil, containerNotInPod
}

func newCertPool(certs []*x509.Certificate) *x509.CertPool {
	certPool := x509.NewCertPool()
	for _, cert := range certs {
		certPool.AddCert(cert)
	}
	return certPool
}
