package cloudsigma

import (
	"reflect"
	"testing"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
)

func TestStructureServer_expandEnclavePageCaches(t *testing.T) {
	cases := []struct {
		description string
		input       []interface{}
		expected    []cloudsigma.EnclavePageCache
	}{
		{"Nil", nil, []cloudsigma.EnclavePageCache{}},
		{
			"SingleEPC",
			[]interface{}{1024},
			[]cloudsigma.EnclavePageCache{{Size: 1024}},
		},
		{
			"MultipleEPCs",
			[]interface{}{1024, 2048},
			[]cloudsigma.EnclavePageCache{
				{Size: 1024},
				{Size: 2048},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			got := expandEnclavePageCaches(c.input)
			if len(got) != len(c.expected) {
				t.Fatalf("expected length: %#v: got: %#v", len(got), len(c.expected))
			}
			if !reflect.DeepEqual(got, c.expected) {
				t.Fatalf("expected: %#v, got: %#v", c.expected, got)
			}
		})
	}
}

func TestStructureServer_flattenEnclavePageCaches(t *testing.T) {
	cases := []struct {
		description string
		input       []cloudsigma.EnclavePageCache
		expected    []interface{}
	}{
		{"Nil", nil, []interface{}{}},
		{
			"SingleEPC",
			[]cloudsigma.EnclavePageCache{{Size: 1024}},
			[]interface{}{1024},
		},
		{
			"MultipleEPCs",
			[]cloudsigma.EnclavePageCache{
				{Size: 1024},
				{Size: 2048},
			},
			[]interface{}{1024, 2048},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			got := flattenEnclavePageCaches(c.input)
			if len(got) != len(c.expected) {
				t.Fatalf("expected length: %#v: got: %#v", len(got), len(c.expected))
			}
			if !reflect.DeepEqual(got, c.expected) {
				t.Fatalf("expected: %#v, got: %#v", c.expected, got)
			}
		})
	}
}

func TestServerPutBody(t *testing.T) {
	live := []byte(`{"uuid":"u","name":"old","cpu":2000,"cpu_model":"x","mem":9007199254740993,
		"nics":[{"mac":"22:aa:00:00:00:01","vlan":{"uuid":"v"}}],
		"drives":[{"dev_channel":"0:0","drive":{"uuid":"boot"}},{"dev_channel":"0:1","drive":{"uuid":"csi"}}],
		"tags":[{"uuid":"t"}],"meta":{"ssh_public_key":"k","stale":"1"}}`)
	body, err := serverPutBody(live,
		map[string]interface{}{"name": "new", "tags": []cloudsigma.Tag{}},
		map[string]interface{}{"private_ip": "10.10.0.10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(body["nics"].([]interface{})) != 1 || len(body["drives"].([]interface{})) != 2 || body["cpu_model"] != "x" {
		t.Errorf("unchanged fields were not carried over: %v", body)
	}
	if body["name"] != "new" || len(body["tags"].([]cloudsigma.Tag)) != 0 {
		t.Errorf("set fields were not applied: %v", body)
	}
	wantMeta := map[string]interface{}{"ssh_public_key": "k", "private_ip": "10.10.0.10"}
	if !reflect.DeepEqual(body["meta"], wantMeta) {
		t.Errorf("meta = %v, want %v", body["meta"], wantMeta)
	}
	if got := body["mem"].(interface{ String() string }).String(); got != "9007199254740993" {
		t.Errorf("mem rounded to %s", got)
	}
	if _, err := serverPutBody([]byte(`{}`), nil, nil); err == nil {
		t.Error("a response without a uuid must not be PUT back")
	}
}

func TestFlattenServerDrives(t *testing.T) {
	if _, ok := flattenServerDrives(nil); ok {
		t.Fatal("nil drives must leave state untouched")
	}
	if got, ok := flattenServerDrives([]cloudsigma.ServerDrive{}); !ok || len(got) != 0 {
		t.Fatalf("empty list must clear state, got %v %v", got, ok)
	}
	if _, ok := flattenServerDrives([]cloudsigma.ServerDrive{{Device: "virtio"}}); ok {
		t.Fatal("drive without UUID must leave state untouched")
	}
	got, ok := flattenServerDrives([]cloudsigma.ServerDrive{{Device: "scsi", Drive: &cloudsigma.Drive{UUID: "u1"}}})
	want := []interface{}{map[string]interface{}{"uuid": "u1", "device": "scsi"}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v %v, want %v", got, ok, want)
	}
}
