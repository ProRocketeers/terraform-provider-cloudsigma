package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
)

var (
	_ datasource.DataSource              = (*serverAvailabilityGroupsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*serverAvailabilityGroupsDataSource)(nil)
)

// serverAvailabilityGroupsDataSource lists which servers share a physical host.
type serverAvailabilityGroupsDataSource struct {
	client *cloudsigma.Client
}

type serverAvailabilityGroupsDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	Groups types.List   `tfsdk:"groups"`
}

func NewServerAvailabilityGroupsDataSource() datasource.DataSource {
	return &serverAvailabilityGroupsDataSource{}
}

func (d *serverAvailabilityGroupsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = "cloudsigma_server_availability_groups"
}

func (d *serverAvailabilityGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: `
The server availability groups data source lists which servers currently run on the same physical host.
Pair it with the server ` + "`avoid_name_prefix`" + ` hint to check that the hint was honoured.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Static identifier of the data source.",
				Computed:            true,
			},
			"groups": schema.ListAttribute{
				MarkdownDescription: "One entry per physical host: the UUIDs of the servers sharing it.",
				ElementType:         types.ListType{ElemType: types.StringType},
				Computed:            true,
			},
		},
	}
}

func (d *serverAvailabilityGroupsDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*cloudsigma.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unconfigured CloudSigma client",
			"Please report this issue to the provider developers.",
		)
		return
	}

	d.client = client
}

func (d *serverAvailabilityGroupsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, response *datasource.ReadResponse) {
	// The SDK has no call for this endpoint; the body is a bare [][]string.
	req, err := d.client.NewRequest(http.MethodGet, "servers/availability_groups/", nil)
	if err != nil {
		response.Diagnostics.AddError("Unable to build availability groups request", err.Error())
		return
	}
	var groups [][]string
	if _, err := d.client.Do(ctx, req, &groups); err != nil {
		response.Diagnostics.AddError("Unable to read server availability groups", fmt.Sprintf("%s", err))
		return
	}

	list, diags := types.ListValueFrom(ctx, types.ListType{ElemType: types.StringType}, groups)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	data := serverAvailabilityGroupsDataSourceModel{
		ID:     types.StringValue("server_availability_groups"),
		Groups: list,
	}
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
