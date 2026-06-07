package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// Executor runs commands inside Kubernetes pods via the API server exec API (SPDY).
type Executor struct {
	client  kubernetes.Interface
	restCfg *rest.Config
}

// NewExecutor creates an Executor from in-cluster config.
func NewExecutor() (*Executor, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	return &Executor{client: cs, restCfg: cfg}, nil
}

// ExecOptions specifies the pod exec parameters.
type ExecOptions struct {
	Namespace string
	Pod       string
	Container string
	Command   []string
	Stdin     io.Reader
}

// ExecResult contains stdout/stderr output from a pod exec call.
type ExecResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// Exec runs a command in the specified pod and returns combined output.
func (e *Executor) Exec(ctx context.Context, opts ExecOptions) (*ExecResult, error) {
	req := e.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(opts.Pod).
		Namespace(opts.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.Container,
			Command:   opts.Command,
			Stdin:     opts.Stdin != nil,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	spdy, err := remotecommand.NewSPDYExecutor(e.restCfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("create SPDY executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	streamErr := spdy.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	result := &ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if streamErr != nil {
		return result, fmt.Errorf("exec stream: %w", streamErr)
	}
	return result, nil
}
