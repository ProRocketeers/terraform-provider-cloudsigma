package cloudsigma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
)

func resourceCloudSigmaServer() *schema.Resource {
	return &schema.Resource{
		Description: `
The server resource allows you to manage CloudSigma servers.
`,

		CreateContext: resourceCloudSigmaServerCreate,
		ReadContext:   resourceCloudSigmaServerRead,
		UpdateContext: resourceCloudSigmaServerUpdate,
		DeleteContext: resourceCloudSigmaServerDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
		},

		SchemaVersion: 0,

		Schema: map[string]*schema.Schema{
			"cpu": {
				Description:      "Server's CPU Clock speed measured in MHz.",
				Type:             schema.TypeInt,
				Required:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.IntBetween(250, 124000)), // 250MHz - 100GHz
			},

			"drive": {
				Description: "Drive attached to the server on creation." +
					"The server will boot from the first defined drive in this resource, which get `boot_order = 1`.",
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Description: "The UUID of the drive.",
							Type:        schema.TypeString,
							Required:    true,
						},
						"device": {
							Default:          "virtio",
							Description:      "Device emulation type. Valid values: `ide`, `virtio`(default), `scsi`.",
							Type:             schema.TypeString,
							Optional:         true,
							ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"ide", "virtio", "scsi"}, false)),
						},
					},
				},
			},

			"enclave_page_caches": {
				Description: "SGX enclaves defined with its size in bytes.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeInt,
				},
			},

			"ipv4_address": {
				Description: "The IPv4 address.",
				Type:        schema.TypeString,
				Computed:    true,
			},

			"memory": {
				Description: "Server's RAM measured in bytes.",
				Type:        schema.TypeInt,
				Required:    true,
			},

			"meta": {
				Description: "The field can be used to store arbitrary information in key-value form.",
				Type:        schema.TypeMap,
				Optional:    true,
				Elem: &schema.Schema{
					Type:     schema.TypeString,
					Required: true,
					ValidateDiagFunc: validation.ToDiagFunc(validation.StringDoesNotMatch(regexp.MustCompile("^ssh_public_key$"),
						"Do not specify ssh_public_key in the meta. Use ssh_keys property instead.")),
				},
				ValidateDiagFunc: validation.MapKeyLenBetween(0, 32),
			},

			"name": {
				Description: "Human readable name of server.",
				Type:        schema.TypeString,
				Required:    true,
			},

			"network": {
				Description: "Network interface card attached to the server.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"ipv4_address": {
							Description: "The IP address reference. Only used with `static` type.",
							Type:        schema.TypeString,
							Optional:    true,
						},
						"type": {
							Description:      "Configuration type. Valid values: `dhcp`, `static`, `manual`.",
							Type:             schema.TypeString,
							Optional:         true,
							ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"dhcp", "static"}, false)),
						},
						"vlan_uuid": {
							Description: "The UUID of the VLAN reference.",
							Type:        schema.TypeString,
							Optional:    true,
						},
						"mac": {
							Description: "The MAC address of the NIC, assigned by CloudSigma. Useful for DHCP reservations.",
							Type:        schema.TypeString,
							Computed:    true,
						},
					},
				},
			},

			"resource_uri": {
				Description: "The unique resource identifier of the server.",
				Type:        schema.TypeString,
				Computed:    true,
			},

			"smp": {
				Description: "Symmetric Multiprocessing (SMP) i.e. number of CPU cores.",
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
			},

			"ssh_keys": {
				Description: "A list of the SSH key UUIDs to be applied to the server.",
				Type:        schema.TypeSet,
				Optional:    true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.NoZeroValues,
				},
			},

			"tags": {
				Description: "A list of the tags UUIDs to be applied to the server.",
				Type:        schema.TypeSet,
				Optional:    true,
				Computed:    true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.NoZeroValues,
				},
			},

			"avoid_name_prefix": {
				Description: "Ask CloudSigma to start this server on a different physical host than every other " +
					"server whose name starts with this prefix (the `avoid` start hint). Best effort: the start " +
					"succeeds even when the hint cannot be satisfied. Only applied when the provider starts the " +
					"server, so changing it never restarts a running server.",
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.NoZeroValues,
			},

			"vnc_password": {
				Description: "VNC Password to connect to server.",
				Type:        schema.TypeString,
				Required:    true,
			},
		},
	}
}

func resourceCloudSigmaServerCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*cloudsigma.Client)

	err := validateSMP(d)
	if err != nil {
		return diag.FromErr(err)
	}

	// build create configuration
	createRequest := &cloudsigma.ServerCreateRequest{
		Servers: []cloudsigma.Server{
			{
				CPU:         d.Get("cpu").(int),
				Memory:      d.Get("memory").(int),
				Name:        d.Get("name").(string),
				SMP:         d.Get("smp").(int),
				VNCPassword: d.Get("vnc_password").(string),
			},
		},
	}

	if v, ok := d.GetOk("enclave_page_caches"); ok {
		createRequest.Servers[0].EnclavePageCaches = expandEnclavePageCaches(v.([]interface{}))
	}

	if ns, ok := d.GetOk("network"); ok {
		networks := ns.([]interface{})
		createRequest.Servers[0].NICs = make([]cloudsigma.ServerNIC, len(networks))

		for i, n := range networks {
			if n == nil {
				return diag.Errorf("defined network argument is empty")
			}
			network := n.(map[string]interface{})
			networkType := network["type"].(string)
			networkAddress := network["ipv4_address"].(string)
			networkVlan := network["vlan_uuid"].(string)

			if networkType == "static" && networkAddress == "" {
				return diag.Errorf("network address cannot be empty if type is static")
			}
			if networkType != "" && networkVlan != "" {
				return diag.Errorf("cannot assign both network type and vlan")
			}

			if networkType == "static" {
				conf := &cloudsigma.ServerIPConfiguration{
					Type:      networkType,
					IPAddress: &cloudsigma.IP{UUID: networkAddress},
				}
				createRequest.Servers[0].NICs[i].IP4Configuration = conf
			} else if networkType == "dhcp" {
				conf := &cloudsigma.ServerIPConfiguration{
					Type: networkType,
				}
				createRequest.Servers[0].NICs[i].IP4Configuration = conf
			} else if networkVlan != "" {
				vlan := &cloudsigma.VLAN{
					UUID: networkVlan,
				}
				createRequest.Servers[0].NICs[i].VLAN = vlan
			}
		}
	} else {
		createRequest.Servers[0].NICs = []cloudsigma.ServerNIC{
			{
				IP4Configuration: &cloudsigma.ServerIPConfiguration{Type: "dhcp"},
				Model:            "virtio",
			},
		}
	}

	if v, ok := d.GetOk("ssh_keys"); ok {
		createRequest.Servers[0].PublicKeys = expandSSHKeys(v.(*schema.Set).List())
	}

	if v, ok := d.GetOk("tags"); ok {
		createRequest.Servers[0].Tags = expandTags(v.(*schema.Set).List())
	}

	if v, ok := d.GetOk("meta"); ok {
		m := v.(map[string]interface{})
		if createRequest.Servers[0].Meta == nil {
			createRequest.Servers[0].Meta = make(map[string]interface{})
		}

		for k, val := range m {
			createRequest.Servers[0].Meta[k] = val
		}
	}

	log.Printf("[DEBUG] Server create configuration: %v", createRequest)
	servers, _, err := client.Servers.Create(ctx, createRequest)
	if err != nil {
		return diag.FromErr(err)
	}

	server := servers[0]
	log.Printf("[INFO] Server ID: %s", server.UUID)

	// store the resulting UUID so we can look this up later
	d.SetId(server.UUID)

	// attach drives
	if ds, ok := d.GetOk("drive"); ok {
		serverDrives := server.Drives

		drives := ds.([]interface{})
		for _, dr := range drives {
			drive := dr.(map[string]interface{})

			serverDrives = append(serverDrives, cloudsigma.ServerDrive{
				BootOrder:  len(serverDrives) + 1,
				DevChannel: fmt.Sprintf("0:%d", len(serverDrives)+1),
				Device:     drive["device"].(string),
				Drive:      &cloudsigma.Drive{UUID: drive["uuid"].(string)},
			})
		}

		server.Drives = serverDrives
		updateRequest := &cloudsigma.ServerUpdateRequest{
			Server: &server,
		}
		log.Printf("[DEBUG] Server update configuration (attach drives): %v", updateRequest)
		_, _, err := client.Servers.Update(ctx, d.Id(), updateRequest)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	// start server
	err = startServer(ctx, client, d.Id(), d.Get("avoid_name_prefix").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCloudSigmaServerRead(ctx, d, meta)
}

func resourceCloudSigmaServerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*cloudsigma.Client)

	server, resp, err := client.Servers.Get(ctx, d.Id())
	if err != nil {
		// If the server is somehow already destroyed, mark as successfully gone
		if resp != nil && resp.StatusCode == 404 {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	_ = d.Set("cpu", server.CPU)
	_ = d.Set("ipv4_address", findIPv4Address(server, "public"))
	_ = d.Set("memory", server.Memory)
	_ = d.Set("name", server.Name)
	_ = d.Set("resource_uri", server.ResourceURI)
	_ = d.Set("smp", server.SMP)
	_ = d.Set("vnc_password", server.VNCPassword)

	if server.PublicKeys != nil {
		_ = d.Set("ssh_keys", extractSSHKeys(server.PublicKeys))
	}

	if drives, ok := flattenServerDrives(server.Drives); ok {
		if err := d.Set("drive", drives); err != nil {
			return diag.Errorf("error setting server drives: %v", err)
		}
	}

	if server.Meta != nil {
		meta := make(map[string]interface{})
		for k, val := range server.Meta {
			// Ignore ssh_public key as it is managed by ssh_keys property
			if k != "ssh_public_key" {
				meta[k] = val.(string)
			}
		}
		if len(meta) > 0 {
			_ = d.Set("meta", meta)
		}
	}

	if len(server.NICs) > 0 {
		var networks []map[string]interface{}
		for _, nws := range server.NICs {
			nw := make(map[string]interface{})
			if nws.IP4Configuration != nil {
				nw["type"] = nws.IP4Configuration.Type
				if nws.IP4Configuration.IPAddress != nil {
					nw["ipv4_address"] = nws.IP4Configuration.IPAddress.UUID
				}
			}
			if nws.VLAN != nil {
				nw["vlan_uuid"] = nws.VLAN.UUID
			}
			nw["mac"] = nws.MACAddress
			networks = append(networks, nw)
		}
		if err := d.Set("network", networks); err != nil {
			return diag.Errorf("error setting network: %v", err)
		}
	}

	if err := d.Set("enclave_page_caches", flattenEnclavePageCaches(server.EnclavePageCaches)); err != nil {
		return diag.Errorf("error setting Server EPC - error: %#v", err)
	}

	if err := d.Set("tags", flattenTags(server.Tags)); err != nil {
		return diag.Errorf("error setting Server tags - error: %#v", err)
	}

	return nil
}

func resourceCloudSigmaServerUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*cloudsigma.Client)

	// avoid_name_prefix is a start-time hint only; it never reaches the server
	// definition, so a change to it alone must not send an update.
	if !d.HasChangesExcept("avoid_name_prefix") {
		return resourceCloudSigmaServerRead(ctx, d, meta)
	}

	// Note that if a server is running, only name, meta, and tags fields can be changed
	// and all other changes to the definition of a running server will be ignored.
	needRestart := d.HasChangesExcept("name", "meta", "tags", "avoid_name_prefix")

	err := validateSMP(d)
	if err != nil {
		return diag.FromErr(err)
	}

	// set holds only the fields this update writes. They are overlaid on the
	// server as the API returns it (serverPutBody): the PUT replaces the whole
	// object, so a field left out of the body is removed from the server.
	set := map[string]interface{}{
		"cpu":  d.Get("cpu").(int),
		"mem":  d.Get("memory").(int),
		"name": d.Get("name").(string),
		"epcs": expandEnclavePageCaches(d.Get("enclave_page_caches").([]interface{})),
		"tags": expandTags(d.Get("tags").(*schema.Set).List()),
	}
	if v := d.Get("smp").(int); v != 0 {
		set["smp"] = v
	}
	if v := d.Get("vnc_password").(string); v != "" {
		set["vnc_password"] = v
	}

	if d.HasChange("drive") {
		serverDrives := make([]cloudsigma.ServerDrive, 0)

		drives := d.Get("drive").([]interface{})
		for _, dr := range drives {
			drive := dr.(map[string]interface{})

			serverDrives = append(serverDrives, cloudsigma.ServerDrive{
				BootOrder:  len(serverDrives) + 1,
				DevChannel: fmt.Sprintf("0:%d", len(serverDrives)+1),
				Device:     drive["device"].(string),
				Drive:      &cloudsigma.Drive{UUID: drive["uuid"].(string)},
			})
		}

		set["drives"] = serverDrives
	}

	configMeta := d.Get("meta").(map[string]interface{})

	if d.HasChange("network") {
		serverNICs := make([]cloudsigma.ServerNIC, 0)

		networks := d.Get("network").([]interface{})
		for _, n := range networks {
			network := n.(map[string]interface{})
			networkType := network["type"].(string)
			networkAddress := network["ipv4_address"].(string)
			networkVlan := network["vlan_uuid"].(string)

			if networkType == "static" && networkAddress == "" {
				return diag.Errorf("network address cannot be empty if type is static")
			}
			if networkType != "" && networkVlan != "" {
				return diag.Errorf("cannot assign both network type and vlan")
			}

			if networkType == "static" {
				serverNICs = append(serverNICs, cloudsigma.ServerNIC{
					IP4Configuration: &cloudsigma.ServerIPConfiguration{
						Type:      networkType,
						IPAddress: &cloudsigma.IP{UUID: networkAddress},
					},
				})
			} else if networkType == "dhcp" {
				serverNICs = append(serverNICs, cloudsigma.ServerNIC{
					IP4Configuration: &cloudsigma.ServerIPConfiguration{
						Type: networkType,
					},
				})
			} else if networkVlan != "" {
				serverNICs = append(serverNICs, cloudsigma.ServerNIC{
					VLAN: &cloudsigma.VLAN{
						UUID: networkVlan,
					},
				})
			}
		}

		set["nics"] = serverNICs
	}

	if needRestart {
		err = stopServer(ctx, client, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
	}

	// Read after the stop and right before the PUT, to keep the window in which
	// another writer (the CSI driver attaching a drive) can be overwritten short.
	path := fmt.Sprintf("servers/%s/", d.Id())
	req, err := client.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	var live bytes.Buffer
	if _, err = client.Do(ctx, req, &live); err != nil {
		return diag.FromErr(err)
	}
	body, err := serverPutBody(live.Bytes(), set, configMeta)
	if err != nil {
		return diag.FromErr(err)
	}
	log.Printf("[DEBUG] Server update sets: %v", set)
	req, err = client.NewRequest(http.MethodPut, path, body)
	if err != nil {
		return diag.FromErr(err)
	}
	if _, err = client.Do(ctx, req, nil); err != nil {
		return diag.FromErr(err)
	}

	if needRestart {
		err = startServer(ctx, client, d.Id(), d.Get("avoid_name_prefix").(string))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceCloudSigmaServerRead(ctx, d, meta)
}

// serverPutBody builds the body of a server PUT: the server exactly as the API
// returned it, with set laid over it and meta replaced by configMeta.
//
// The PUT replaces the whole object, also on a running server. A body built from
// the configuration alone leaves out everything it does not model - above all
// the NICs and drives when they did not change - and the API then removes them.
// That is how a meta-only change took a running guest off the network.
//
// ssh_public_key is kept from the live meta: it is managed by ssh_keys and is
// never in the configuration.
func serverPutBody(live []byte, set, configMeta map[string]interface{}) (map[string]interface{}, error) {
	// json.Number: a float64 would round integers above 2^53 on the way back.
	decoder := json.NewDecoder(bytes.NewReader(live))
	decoder.UseNumber()
	var body map[string]interface{}
	if err := decoder.Decode(&body); err != nil {
		return nil, fmt.Errorf("error decoding server for update: %s", err)
	}
	if body["uuid"] == nil {
		return nil, fmt.Errorf("error reading server for update: response has no uuid")
	}
	for k, v := range set {
		body[k] = v
	}
	meta := make(map[string]interface{}, len(configMeta)+1)
	if liveMeta, ok := body["meta"].(map[string]interface{}); ok {
		if key, ok := liveMeta["ssh_public_key"]; ok {
			meta["ssh_public_key"] = key
		}
	}
	for k, v := range configMeta {
		meta[k] = v
	}
	body["meta"] = meta
	return body, nil
}

func resourceCloudSigmaServerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*cloudsigma.Client)

	server, resp, err := client.Servers.Get(ctx, d.Id())
	if err != nil {
		// handle remotely destroyed server
		if resp != nil && resp.StatusCode == 404 {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	// Stop server
	err = stopServer(ctx, client, server.UUID)
	if err != nil {
		return diag.Errorf("error stopping server: %s", err)
	}

	// Delete server
	_, err = client.Servers.Delete(ctx, server.UUID)
	if err != nil {
		return diag.Errorf("error deleting server: %s", err)
	}

	d.SetId("")

	return nil
}

func expandSSHKeys(sshKeys []interface{}) []cloudsigma.Keypair {
	expandedSshKeys := make([]cloudsigma.Keypair, len(sshKeys))
	for i, s := range sshKeys {
		sshKey := s.(string)
		var expandedSshKey cloudsigma.Keypair
		expandedSshKey.UUID = sshKey
		expandedSshKeys[i] = expandedSshKey
	}

	return expandedSshKeys
}

func extractSSHKeys(serverSSHKeys []cloudsigma.Keypair) []interface{} {
	extractedSshKeys := make([]interface{}, len(serverSSHKeys))
	for i, v := range serverSSHKeys {
		extractedSshKeys[i] = v.UUID
	}

	return extractedSshKeys
}

// flattenServerDrives projects the response's attached-drive references into
// the resource state. A missing or malformed drives field is left untouched so
// a partial response cannot erase a known attachment; an explicit empty list
// is still authoritative and clears the state.
func flattenServerDrives(drives []cloudsigma.ServerDrive) ([]interface{}, bool) {
	if drives == nil {
		return nil, false
	}
	result := make([]interface{}, len(drives))
	for i, attached := range drives {
		if attached.Drive == nil || attached.Drive.UUID == "" {
			return nil, false
		}
		result[i] = map[string]interface{}{
			"uuid":   attached.Drive.UUID,
			"device": attached.Device,
		}
	}
	return result, true
}

func findIPv4Address(server *cloudsigma.Server, addrType string) string {
	if server.Runtime == nil {
		return ""
	}

	for _, nic := range server.Runtime.RuntimeNICs {
		if nic.InterfaceType == addrType {
			return nic.IPv4.UUID
		}
	}
	return ""
}

func validateSMP(d *schema.ResourceData) error {
	if v, ok := d.GetOk("smp"); ok {
		smp := v.(int)
		cpu := d.Get("cpu").(int)
		if cpu/smp < 1000 {
			return fmt.Errorf("the minimum amount of cpu per smp is 1000 (currently is %v)", cpu/smp)
		}
	}
	return nil
}

func serverStateRefreshFunc(ctx context.Context, client *cloudsigma.Client, serverUUID string) retry.StateRefreshFunc {
	return func() (interface{}, string, error) {
		server, _, err := client.Servers.Get(ctx, serverUUID)
		if err != nil {
			return nil, "", fmt.Errorf("error retrieving server with uuid %s: %s", serverUUID, err)
		}

		return server, server.Status, nil
	}
}

// startServer starts a stopped server. A non-empty avoidNamePrefix sends the
// start with the avoid hint, listing every other server whose name has that prefix.
func startServer(ctx context.Context, client *cloudsigma.Client, serverUUID, avoidNamePrefix string) error {
	log.Printf("[DEBUG] Starting server (%s)", serverUUID)

	log.Printf("[DEBUG] Checking server status before starting")
	server, _, err := client.Servers.Get(ctx, serverUUID)
	if err != nil {
		return fmt.Errorf("error retrieving server: %s", err)
	}

	if server.Status == "running" {
		log.Printf("[DEBUG] Server (%s) is already running", server.UUID)
		return nil
	}

	if avoidNamePrefix == "" {
		_, _, err = client.Servers.Start(ctx, server.UUID)
	} else {
		err = startServerAvoiding(ctx, client, server.UUID, avoidNamePrefix)
	}
	if err != nil {
		return fmt.Errorf("error starting server: %s", err)
	}
	stateConf := &retry.StateChangeConf{
		Pending:    []string{"stopped", "starting", "unavailable"},
		Target:     []string{"running"},
		Refresh:    serverStateRefreshFunc(ctx, client, server.UUID),
		Timeout:    10 * time.Minute,
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}
	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("error waiting for server (%s) to become running: %s", server.UUID, err)
	}

	return nil
}

func stopServer(ctx context.Context, client *cloudsigma.Client, serverUUID string) error {
	log.Printf("[DEBUG] Stopping server (%s)", serverUUID)

	log.Printf("[DEBUG] Checking server status before stopping")
	server, resp, err := client.Servers.Get(ctx, serverUUID)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil
		}
		return fmt.Errorf("error retrieving server: %s", err)
	}

	if server.Status == "stopped" {
		log.Printf("[DEBUG] Server (%s) is already stopped", server.UUID)
		return nil
	}

	_, _, err = client.Servers.Stop(context.Background(), server.UUID)
	if err != nil {
		return fmt.Errorf("error stopping server: %s", err)
	}
	stateConf := &retry.StateChangeConf{
		Pending:    []string{"running", "stopping", "unavailable"},
		Target:     []string{"stopped"},
		Refresh:    serverStateRefreshFunc(ctx, client, server.UUID),
		Timeout:    10 * time.Minute,
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}
	if _, err := stateConf.WaitForStateContext(context.Background()); err != nil {
		return fmt.Errorf("error waiting for server (%s) to become stopped: %s", server.UUID, err)
	}

	return nil
}

// avoidUUIDs returns every server except self whose name starts with prefix,
// sorted by name: the API treats the avoid list as ordered preference.
func avoidUUIDs(servers []cloudsigma.Server, self, prefix string) []string {
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	uuids := make([]string, 0)
	for _, s := range servers {
		if s.UUID != self && strings.HasPrefix(s.Name, prefix) {
			uuids = append(uuids, s.UUID)
		}
	}
	return uuids
}

// startServerAvoiding sends the start action with the avoid hint. The SDK's
// Start takes no query parameters, so the request is built by hand.
func startServerAvoiding(ctx context.Context, client *cloudsigma.Client, serverUUID, prefix string) error {
	// limit=0 disables paging; the default page (20) would silently drop peers.
	req, err := client.NewRequest(http.MethodGet, "servers/?limit=0", nil)
	if err != nil {
		return err
	}
	var list struct {
		Objects []cloudsigma.Server `json:"objects"`
	}
	if _, err := client.Do(ctx, req, &list); err != nil {
		return fmt.Errorf("error listing servers for avoid: %s", err)
	}

	avoid := avoidUUIDs(list.Objects, serverUUID, prefix)
	path := fmt.Sprintf("servers/%s/action/?do=start", serverUUID)
	if len(avoid) > 0 {
		path += "&avoid=" + strings.Join(avoid, ",")
	}
	log.Printf("[DEBUG] Starting server (%s) avoiding %v", serverUUID, avoid)

	req, err = client.NewRequest(http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(ctx, req, nil)
	return err
}
