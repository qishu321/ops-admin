package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/model"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/gorilla/websocket"
)

const maxK8sPodUploadSize int64 = 50 * 1024 * 1024

type k8sTerminalMessage struct {
	Operation string `json:"operation"`
	Data      any    `json:"data,omitempty"`
}

type k8sTerminalSizeData struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type k8sTerminalSizeQueue struct {
	ch chan remotecommand.TerminalSize
}

func newK8sTerminalSizeQueue(rows int, cols int) *k8sTerminalSizeQueue {
	queue := &k8sTerminalSizeQueue{
		ch: make(chan remotecommand.TerminalSize, 8),
	}
	queue.Push(rows, cols)
	return queue
}

func (q *k8sTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return &size
}

func (q *k8sTerminalSizeQueue) Push(rows int, cols int) {
	if q == nil || rows <= 0 || cols <= 0 {
		return
	}
	size := remotecommand.TerminalSize{
		Width:  uint16(cols),
		Height: uint16(rows),
	}
	select {
	case q.ch <- size:
	default:
		select {
		case <-q.ch:
		default:
		}
		q.ch <- size
	}
}

func (q *k8sTerminalSizeQueue) Close() {
	if q == nil {
		return
	}
	close(q.ch)
}

type k8sTerminalOutput struct {
	conn      *websocket.Conn
	writeMu   *sync.Mutex
	readyOnce *sync.Once
}

func (w *k8sTerminalOutput) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if w.readyOnce != nil {
		var readyErr error
		w.readyOnce.Do(func() {
			readyErr = w.conn.WriteJSON(k8sTerminalMessage{
				Operation: "status",
				Data: map[string]any{
					"state":   "connected",
					"message": "Pod 终端已连接",
				},
			})
		})
		if readyErr != nil {
			return 0, readyErr
		}
	}
	if err := w.conn.WriteJSON(k8sTerminalMessage{
		Operation: "stdout",
		Data:      string(p),
	}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *Service) GetK8sPodContainers(clusterID uint, namespace string, podName string) ([]string, error) {
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return nil, err
	}
	config, _, err := s.k8sRESTConfigForCluster(cluster)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	containers := make([]string, 0, len(pod.Spec.Containers))
	for _, item := range pod.Spec.Containers {
		containers = append(containers, item.Name)
	}
	return containers, nil
}

// UploadK8sPodFile streams one file through the Kubernetes exec channel.  The
// file never becomes a cluster-side API object and is written via a temporary
// sibling before the final rename, so readers cannot observe a partial file.
func (s *Service) UploadK8sPodFile(clusterID uint, namespace string, podName string, container string, directory string, filename string, size int64, content io.Reader) (map[string]any, error) {
	directory = path.Clean(strings.TrimSpace(directory))
	filename = strings.TrimSpace(filename)
	if clusterID == 0 || strings.TrimSpace(namespace) == "" || strings.TrimSpace(podName) == "" || !strings.HasPrefix(directory, "/") {
		return nil, errors.New("invalid pod upload target")
	}
	if filename == "" || filename == "." || filename == ".." || path.Base(filename) != filename || strings.ContainsAny(filename, "\\\x00\r\n") {
		return nil, errors.New("invalid upload file name")
	}
	if size < 0 || size > maxK8sPodUploadSize {
		return nil, errors.New("单个文件不能超过 50 MB")
	}

	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return nil, err
	}
	config, cleanup, err := s.k8sSPDYConfigForCluster(cluster)
	if err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	defer cleanup()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	targetContainer := chooseK8sContainerName(pod, container)
	if targetContainer == "" {
		return nil, errors.New("pod container not found")
	}

	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: targetContainer,
		Command:   []string{"/bin/sh", "-c", `set -eu; dir="$1"; name="$2"; test -d "$dir"; test -w "$dir"; tmp="$dir/.${name}.ops-upload-$$"; trap 'rm -f "$tmp"' EXIT HUP INT TERM; cat > "$tmp"; mv -f "$tmp" "$dir/$name"; trap - EXIT`, "--", directory, filename},
		Stdin:     true, Stdout: true, Stderr: true, TTY: false,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: io.LimitReader(content, maxK8sPodUploadSize+1), Stdout: &stdout, Stderr: &stderr, Tty: false}); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("上传到容器失败：%s", message)
		}
		return nil, err
	}
	return map[string]any{"directory": directory, "filename": filename, "size": size, "container": targetContainer}, nil
}

func (s *Service) OpenK8sPodTerminal(clusterID uint, namespace string, podName string, container string, command string, rows int, cols int, conn *websocket.Conn) error {
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return err
	}
	config, cleanup, err := s.k8sSPDYConfigForCluster(cluster)
	if err != nil {
		return errors.New(k8sClusterConnectError)
	}
	defer cleanup()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.New(k8sClusterConnectError)
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	targetContainer := chooseK8sContainerName(pod, container)
	if targetContainer == "" {
		return errors.New("pod container not found")
	}

	cmd := normalizeK8sTerminalCommand(command)
	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: targetContainer,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdinReader, stdinWriter := io.Pipe()
	defer stdinReader.Close()
	defer stdinWriter.Close()

	sizeQueue := newK8sTerminalSizeQueue(rows, cols)
	defer sizeQueue.Close()

	writeMu := &sync.Mutex{}
	readyOnce := &sync.Once{}
	streamErrCh := make(chan error, 1)
	readErrCh := make(chan error, 1)

	go func() {
		streamErrCh <- executor.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:             stdinReader,
			Stdout:            &k8sTerminalOutput{conn: conn, writeMu: writeMu, readyOnce: readyOnce},
			Stderr:            &k8sTerminalOutput{conn: conn, writeMu: writeMu, readyOnce: readyOnce},
			Tty:               true,
			TerminalSizeQueue: sizeQueue,
		})
	}()

	go func() {
		defer stdinWriter.Close()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				readErrCh <- err
				return
			}

			var message k8sTerminalMessage
			if err := json.Unmarshal(payload, &message); err != nil {
				if _, writeErr := stdinWriter.Write(payload); writeErr != nil {
					readErrCh <- writeErr
					return
				}
				continue
			}

			switch message.Operation {
			case "stdin":
				text, _ := message.Data.(string)
				if text == "" {
					continue
				}
				if _, err := stdinWriter.Write([]byte(text)); err != nil {
					readErrCh <- err
					return
				}
			case "resize":
				var size k8sTerminalSizeData
				if raw, err := json.Marshal(message.Data); err == nil && json.Unmarshal(raw, &size) == nil {
					sizeQueue.Push(int(size.Rows), int(size.Cols))
				}
			}
		}
	}()

	select {
	case err := <-streamErrCh:
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	case err := <-readErrCh:
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			return nil
		}
		return nil
	}
}

// k8sSPDYConfigForCluster uses a short-lived localhost tunnel for gateway
// clusters. client-go's SPDY transport does not honor rest.Config.Dial, so the
// regular gateway REST config is insufficient for pod exec/attach requests.
func (s *Service) k8sSPDYConfigForCluster(cluster model.K8sCluster) (*rest.Config, func(), error) {
	config, _, err := s.k8sRESTConfigForCluster(cluster)
	if err != nil {
		return nil, func() {}, err
	}
	if normalizeConnectionMode(cluster.ConnectionMode) != "gateway" || cluster.GatewayID == nil || *cluster.GatewayID == 0 {
		return config, func() {}, nil
	}

	targetAddress, err := k8sAPITargetAddress(config.Host)
	if err != nil {
		return nil, func() {}, err
	}
	tunnelAddress, cleanup, err := s.startGatewayTunnel(*cluster.GatewayID, targetAddress)
	if err != nil {
		return nil, func() {}, err
	}
	tunnelConfig, _, err := k8sSPDYConfigThroughGateway(config, tunnelAddress)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return tunnelConfig, cleanup, nil
}

func k8sSPDYConfigThroughGateway(config *rest.Config, tunnelAddress string) (*rest.Config, string, error) {
	if config == nil {
		return nil, "", errors.New("Kubernetes REST config is nil")
	}
	endpoint, err := url.Parse(config.Host)
	if err != nil || endpoint.Scheme == "" || endpoint.Hostname() == "" {
		return nil, "", fmt.Errorf("invalid Kubernetes API server URL: %s", config.Host)
	}
	targetAddress, err := k8sAPITargetAddress(config.Host)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(tunnelAddress) == "" {
		return nil, "", errors.New("Kubernetes gateway tunnel address is empty")
	}

	tunnelEndpoint := *endpoint
	tunnelEndpoint.Host = tunnelAddress
	result := rest.CopyConfig(config)
	result.Host = tunnelEndpoint.String()
	result.Dial = nil
	if result.TLSClientConfig.ServerName == "" {
		result.TLSClientConfig.ServerName = endpoint.Hostname()
	}
	return result, targetAddress, nil
}

func k8sAPITargetAddress(rawURL string) (string, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Hostname() == "" {
		return "", fmt.Errorf("invalid Kubernetes API server URL: %s", rawURL)
	}
	port := endpoint.Port()
	if port == "" {
		switch strings.ToLower(endpoint.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return "", fmt.Errorf("unsupported Kubernetes API server scheme: %s", endpoint.Scheme)
		}
	}
	return net.JoinHostPort(endpoint.Hostname(), port), nil
}

func (s *Service) k8sRESTConfigForCluster(cluster model.K8sCluster) (*rest.Config, func(), error) {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(cluster.KubeConfig))
	if err != nil {
		return nil, func() {}, err
	}
	endpoint, err := url.Parse(config.Host)
	if err != nil || endpoint.Hostname() == "" {
		return nil, func() {}, fmt.Errorf("invalid Kubernetes API server URL: %s", config.Host)
	}
	if normalizeConnectionMode(cluster.ConnectionMode) == "gateway" && cluster.GatewayID != nil && *cluster.GatewayID > 0 {
		if isLoopbackK8sAPIHost(endpoint.Hostname()) {
			return nil, func() {}, fmt.Errorf("Kubernetes API server is configured as %s, which is a stale local tunnel address; restore the original API server address in kubeconfig", config.Host)
		}
		gatewayID := *cluster.GatewayID
		// Standard Kubernetes HTTP requests honor rest.Config.Dial. SPDY-based
		// exec/attach requests are handled separately by k8sSPDYConfigForCluster.
		config.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			return s.dialGatewayTarget(gatewayID, network, address)
		}
		return config, func() {}, nil
	}
	return config, func() {}, nil
}

func isLoopbackK8sAPIHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func chooseK8sContainerName(pod *corev1.Pod, container string) string {
	name := Trimmed(container)
	if name != "" {
		for _, item := range pod.Spec.Containers {
			if item.Name == name {
				return item.Name
			}
		}
	}
	if len(pod.Spec.Containers) > 0 {
		return pod.Spec.Containers[0].Name
	}
	return ""
}

func normalizeK8sTerminalCommand(command string) []string {
	switch Trimmed(command) {
	case "", "sh", "/bin/sh":
		// Kubernetes exec receives stdin through a pipe. Be explicit about
		// interactive mode so the shell enables line editing and Tab completion.
		// Prefer bash when the image provides it, with a portable sh fallback.
		// The marker is consumed by the browser terminal and keeps the upload
		// destination in sync with the interactive shell's current directory.
		return []string{"/bin/sh", "-c", `if command -v bash >/dev/null 2>&1; then export PROMPT_COMMAND='printf "\036OPS_ADMIN_CWD:%s\037" "$PWD"'; exec bash -i; else export PS1='$(printf "\036OPS_ADMIN_CWD:%s\037" "$PWD")$ '; exec /bin/sh -i; fi`}
	case "bash", "/bin/bash":
		return []string{"/bin/bash", "-i"}
	default:
		return []string{Trimmed(command)}
	}
}
