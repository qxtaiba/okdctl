package okd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

const fakeOCConvertingOneHostNetworkController = `#!/bin/sh
echo "$KUBECONFIG|$1 $2" >> "$OC_INVOCATION_LOG"
case "$1 $2" in
"get ingresscontroller")
  if [ -e "$OC_INVOCATION_LOG.listed" ]; then echo "fake: second list fails" >&2; exit 1; fi
  : > "$OC_INVOCATION_LOG.listed"
  echo '{"items":[{"metadata":{"name":"default","namespace":"openshift-ingress-operator"},"spec":{},"status":{"domain":"apps.example.test"}}]}'
  ;;
"get namespace"|"get ipaddresspool") echo present ;;
"get deployment"|"delete ingresscontroller") ;;
"create -f") cat > /dev/null ;;
*) exit 1 ;;
esac
`

const fakeOCFailingEveryCall = `#!/bin/sh
echo "$KUBECONFIG|$1 $2" >> "$OC_INVOCATION_LOG"
exit 1
`

func installFakeOCLoggingKubeconfig(t *testing.T, script string) (invocationLog string) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", script)
	invocationLog = filepath.Join(t.TempDir(), "oc.log")
	t.Setenv("OC_INVOCATION_LOG", invocationLog)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "ambient-kubeconfig"))
	return invocationLog
}

func updateIngressTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "kubeconfig-pin-test"
	return cfg
}

func projectKubeconfigPath(root string) string {
	return workspace.KubeconfigPath(workspace.ClusterConfigDir(workspace.WorkDir(root)))
}

func TestUpdateIngress_PinsEveryOcCallToProjectKubeconfig(t *testing.T) {
	root := t.TempDir()
	projectKubeconfig := projectKubeconfigPath(root)
	if err := os.MkdirAll(filepath.Dir(projectKubeconfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectKubeconfig, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invocationLog := installFakeOCLoggingKubeconfig(t, fakeOCConvertingOneHostNetworkController)

	p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
	_, err := p.UpdateIngress(t.Context(), updateIngressTestConfig(), postinstall.UpdateIngressOptions{
		ConfirmConversion: func([]string) bool { return true },
	})
	if err == nil || !strings.Contains(err.Error(), "re-discover IngressControllers after conversion") {
		t.Fatalf("err = %v; want the run to stop at the fake's failing second list", err)
	}

	logged, err := os.ReadFile(invocationLog)
	if err != nil {
		t.Fatalf("oc was never invoked: %v", err)
	}
	var subcommands []string
	for _, line := range strings.Split(strings.TrimRight(string(logged), "\n"), "\n") {
		kubeconfig, subcommand, _ := strings.Cut(line, "|")
		if kubeconfig != projectKubeconfig {
			t.Errorf("oc %s ran with KUBECONFIG=%q; want the project kubeconfig %q", subcommand, kubeconfig, projectKubeconfig)
		}
		subcommands = append(subcommands, subcommand)
	}
	for _, want := range []string{"get ingresscontroller", "delete ingresscontroller", "create -f"} {
		if !slices.Contains(subcommands, want) {
			t.Errorf("oc %s never ran; invocations: %v", want, subcommands)
		}
	}
}

func TestUpdateIngress_MissingProjectKubeconfigRefusesBeforeOc(t *testing.T) {
	root := t.TempDir()
	invocationLog := installFakeOCLoggingKubeconfig(t, fakeOCFailingEveryCall)

	p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
	_, err := p.UpdateIngress(t.Context(), updateIngressTestConfig(), postinstall.UpdateIngressOptions{})

	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	if want := projectKubeconfigPath(root); !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v; want it to name the missing kubeconfig %s", err, want)
	}
	if _, statErr := os.Stat(invocationLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("oc ran although the project kubeconfig is missing")
	}
}
