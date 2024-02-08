/*
   Copyright The containerd Authors.

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

package shim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	exec "golang.org/x/sys/execabs"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/pkg/atomicfile"
)

type CommandConfig struct {
	Runtime      string
	Address      string
	TTRPCAddress string
	Path         string
	SchedCore    bool
	Args         []string
	Opts         *types.Any
}

// Command returns the shim command with the provided args and configuration
func Command(ctx context.Context, config *CommandConfig) (*exec.Cmd, error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{
		"-namespace", ns,
		"-address", config.Address,
		"-publish-binary", self,
	}
	args = append(args, config.Args...)
	cmd := exec.CommandContext(ctx, config.Runtime, args...)
	cmd.Dir = config.Path
	cmd.Env = append(
		os.Environ(),
		"GOMAXPROCS=2",
		fmt.Sprintf("%s=%s", ttrpcAddressEnv, config.TTRPCAddress),
	)
	if config.SchedCore {
		cmd.Env = append(cmd.Env, "SCHED_CORE=1")
	}
	cmd.SysProcAttr = getSysProcAttr()
	if config.Opts != nil {
		d, err := proto.Marshal(config.Opts)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = bytes.NewReader(d)
	}
	return cmd, nil
}

// BinaryName returns the shim binary name from the runtime name,
// empty string returns means runtime name is invalid
func BinaryName(runtime string) string {
	// runtime name should format like $prefix.name.version
	parts := strings.Split(runtime, ".")
	if len(parts) < 2 {
		return ""
	}

	return fmt.Sprintf(shimBinaryFormat, parts[len(parts)-2], parts[len(parts)-1])
}

// BinaryPath returns the full path for the shim binary from the runtime name,
// empty string returns means runtime name is invalid
func BinaryPath(runtime string) string {
	dir := filepath.Dir(runtime)
	binary := BinaryName(runtime)

	path, err := filepath.Abs(filepath.Join(dir, binary))
	if err != nil {
		return ""
	}

	return path
}

// Connect to the provided address
func Connect(address string, d func(string, time.Duration) (net.Conn, error)) (net.Conn, error) {
	return d(address, 100*time.Second)
}

// WritePidFile writes a pid file atomically
func WritePidFile(path string, pid int) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	f, err := atomicfile.New(path, 0o644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%d", pid)
	if err != nil {
		f.Cancel()
		return err
	}
	return f.Close()
}

// WriteAddress writes a address file atomically
func WriteAddress(path, address string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	f, err := atomicfile.New(path, 0o666)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(address))
	if err != nil {
		f.Cancel()
		return err
	}
	return f.Close()
}

// ErrNoAddress is returned when the address file has no content
var ErrNoAddress = errors.New("no shim address")

// ReadAddress returns the shim's socket address from the path
func ReadAddress(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrNoAddress
	}
	return string(data), nil
}

// chainUnaryServerInterceptors creates a single ttrpc server interceptor from
// a chain of many interceptors executed from first to last.
func chainUnaryServerInterceptors(interceptors ...ttrpc.UnaryServerInterceptor) ttrpc.UnaryServerInterceptor {
	n := len(interceptors)

	// force to use default interceptor in ttrpc
	if n == 0 {
		return nil
	}

	return func(ctx context.Context, unmarshal ttrpc.Unmarshaler, info *ttrpc.UnaryServerInfo, method ttrpc.Method) (interface{}, error) {
		currentMethod := method

		for i := n - 1; i > 0; i-- {
			interceptor := interceptors[i]
			innerMethod := currentMethod

			currentMethod = func(currentCtx context.Context, currentUnmarshal func(interface{}) error) (interface{}, error) {
				return interceptor(currentCtx, currentUnmarshal, info, innerMethod)
			}
		}
		return interceptors[0](ctx, unmarshal, info, currentMethod)
	}
}
