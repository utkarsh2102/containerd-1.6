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

package platforms

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"testing"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestDefault(t *testing.T) {
	major, minor, build := windows.RtlGetNtVersionNumbers()
	expected := imagespec.Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		OSVersion:    fmt.Sprintf("%d.%d.%d", major, minor, build),
		Variant:      cpuVariant(),
	}
	p := DefaultSpec()
	if !reflect.DeepEqual(p, expected) {
		t.Fatalf("default platform not as expected: %#v != %#v", p, expected)
	}

	s := DefaultString()
	if s != Format(p) {
		t.Fatalf("default specifier should match formatted default spec: %v != %v", s, p)
	}
}

func TestMatchComparerMatch(t *testing.T) {
	major, minor, build := windows.RtlGetNtVersionNumbers()
	buildStr := fmt.Sprintf("%d.%d.%d", major, minor, build)
	m := matchComparer{
		defaults: Only(imagespec.Platform{
			Architecture: "amd64",
			OS:           "windows",
		}),
		osVersionPrefix: buildStr,
	}
	for _, test := range []struct {
		platform imagespec.Platform
		match    bool
	}{
		{
			platform: DefaultSpec(),
			match:    true,
		},
		{
			platform: imagespec.Platform{
				Architecture: "amd64",
				OS:           "windows",
				OSVersion:    buildStr + ".1",
			},
			match: true,
		},
		{
			platform: imagespec.Platform{
				Architecture: "amd64",
				OS:           "windows",
				OSVersion:    buildStr + ".2",
			},
			match: true,
		},
		{
			platform: imagespec.Platform{
				Architecture: "amd64",
				OS:           "windows",
				// Use an nonexistent Windows build so we don't get a match. Ws2019's build is 17763/
				OSVersion: "10.0.17762.1",
			},
			match: false,
		},
		{
			platform: imagespec.Platform{
				Architecture: "amd64",
				OS:           "windows",
				// Use an nonexistent Windows build so we don't get a match. Ws2019's build is 17763/
				OSVersion: "10.0.17764.1",
			},
			match: false,
		},
		{
			platform: imagespec.Platform{
				Architecture: "amd64",
				OS:           "windows",
			},
			match: true,
		},
	} {
		assert.Equal(t, test.match, m.Match(test.platform))
	}
}

func TestMatchComparerLess(t *testing.T) {
	m := matchComparer{
		defaults: Only(imagespec.Platform{
			Architecture: "amd64",
			OS:           "windows",
		}),
		osVersionPrefix: "10.0.17763",
	}
	platforms := []imagespec.Platform{
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17764.1",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17763.1",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17763.2",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17762.1",
		},
	}
	expected := []imagespec.Platform{
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17763.2",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17763.1",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17764.1",
		},
		{
			Architecture: "amd64",
			OS:           "windows",
			OSVersion:    "10.0.17762.1",
		},
	}
	sort.SliceStable(platforms, func(i, j int) bool {
		return m.Less(platforms[i], platforms[j])
	})
	assert.Equal(t, expected, platforms)
}
