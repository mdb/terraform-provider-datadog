package fwprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/terraform-providers/terraform-provider-datadog/datadog/internal/validators"
)

func NewDatadogCreateEventAction() action.Action {
	return &createEventAction{}
}

var (
	_ action.Action = (*createEventAction)(nil)
)

type createEventAction struct {
	Api *datadogV2.EventsApi
}

type createEventActionModel struct {
	Title               types.String      `tfsdk:"title"`
	Category            types.String      `tfsdk:"category"`
	AggregationKey      types.String      `tfsdk:"aggregation_key"`
	Host                types.String      `tfsdk:"host"`
	IntegrationID       types.String      `tfsdk:"integration_id"`
	Message             types.String      `tfsdk:"message"`
	Tags                types.List        `tfsdk:"tags"`
	Timestamp           timetypes.RFC3339 `tfsdk:"timestamp"`
	ChangedResourceName types.String      `tfsdk:"changed_resource_name"`
	ChangedResourceType types.String      `tfsdk:"changed_resource_type"`
}

func (a *createEventAction) Schema(_ context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Create a Datadog event",
		Attributes: map[string]schema.Attribute{
			"title": schema.StringAttribute{
				Description: "The title of the event",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(500),
				},
			},
			"category": schema.StringAttribute{
				Description: "Event category identifying the type of event",
				Required:    true,
				Validators: []validator.String{
					validators.NewEnumValidator[validator.String](datadogV2.NewEventCategoryFromValue),
				},
			},
			"aggregation_key": schema.StringAttribute{
				Description: "A string used for aggregation when correlating events",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"host": schema.StringAttribute{
				Description: "Host name associated with the event",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(255),
				},
			},
			"integration_id": schema.StringAttribute{
				Description: "Integration ID sourced from integration manifests",
				Optional:    true,
				Validators: []validator.String{
					validators.NewEnumValidator[validator.String](datadogV2.NewEventPayloadIntegrationIdFromValue),
				},
			},
			"message": schema.StringAttribute{
				Description: "Free formed text associated with the event",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(4000),
				},
			},
			"tags": schema.ListAttribute{
				Description: "A list of tags associated with the event",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.SizeAtMost(100),
				},
			},
			"timestamp": schema.StringAttribute{
				Description: "The timestamp when the event occurred (in ISO 8601).",
				CustomType:  timetypes.RFC3339Type{},
				Optional:    true,
				Validators:  []validator.String{validators.TimeFormatValidator(time.RFC3339)},
			},
			"changed_resource_name": schema.StringAttribute{
				Description: "The name of the changed resource. Required when category is 'change'.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(128),
				},
			},
			"changed_resource_type": schema.StringAttribute{
				Description: "The type of the changed resource (feature_flag or configuration). Required when category is 'change'.",
				Optional:    true,
				Validators: []validator.String{
					validators.NewEnumValidator[validator.String](datadogV2.NewChangeEventCustomAttributesChangedResourceTypeFromValue),
				},
			},
		},
	}
}

func (a *createEventAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*FrameworkProvider)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Action Configure Type",
			fmt.Sprintf("Expected *FrameworkProvider, got: %T", req.ProviderData),
		)
		return
	}

	a.Api = providerData.DatadogApiInstances.GetEventsApiV2()
}

func (a *createEventAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = "create_event"
}

func (a *createEventAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if a.Api == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The Datadog provider was not configured properly. Please ensure the provider block is present and configured.",
		)
		return
	}

	var config createEventActionModel

	// Parse configuration
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	title := config.Title.ValueString()

	tflog.Info(ctx, "Starting Datadog event create invocation action", map[string]any{
		"title":           config.Title,
		"category":        config.Category,
		"aggregation_key": config.AggregationKey,
		"host":            config.Host,
		"integration_id":  config.IntegrationID,
		"message":         config.Message,
		"tags":            config.Tags,
		"timestamp":       config.Timestamp,
	})

	// Send initial progress update
	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Creating Datadog event %s...", title),
	})

	// NOTE: Because these fields' validators guarantees validity, the error path
	// should be unreachable. Errors are ignored, as is idiomatic in other parts of
	// the codebase.
	category, _ := datadogV2.NewEventCategoryFromValue(config.Category.ValueString())
	integrationID, _ := datadogV2.NewEventPayloadIntegrationIdFromValue(config.IntegrationID.ValueString())

	var timestamp *string
	if !config.Timestamp.IsNull() {
		ts, diags := config.Timestamp.ValueRFC3339Time()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		t := ts.Format(time.RFC3339)
		timestamp = &t
	}

	var tags []string
	if !config.Tags.IsNull() {
		resp.Diagnostics.Append(config.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Build category-specific attributes
	var eventPayloadAttributes datadogV2.EventPayloadAttributes
	if *category == datadogV2.EVENTCATEGORY_CHANGE {
		// For change events, we need ChangeEventCustomAttributes with ChangedResource
		if config.ChangedResourceName.IsNull() || config.ChangedResourceType.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"The 'changed_resource_name' and 'changed_resource_type' attributes are required when category is 'change'",
			)
			return
		}

		resourceType, _ := datadogV2.NewChangeEventCustomAttributesChangedResourceTypeFromValue(config.ChangedResourceType.ValueString())

		eventPayloadAttributes = datadogV2.ChangeEventCustomAttributesAsEventPayloadAttributes(
			&datadogV2.ChangeEventCustomAttributes{
				ChangedResource: datadogV2.ChangeEventCustomAttributesChangedResource{
					Name: config.ChangedResourceName.ValueString(),
					Type: *resourceType,
				},
			},
		)
	}

	body := datadogV2.EventCreateRequestPayload{
		Data: datadogV2.EventCreateRequest{
			Type: datadogV2.EVENTCREATEREQUESTTYPE_EVENT,
			Attributes: datadogV2.EventPayload{
				Title:      title,
				Category:   *category,
				Attributes: eventPayloadAttributes,
			},
		},
	}

	aggKey := config.AggregationKey.ValueStringPointer()
	if aggKey != nil {
		body.Data.Attributes.AggregationKey = aggKey
	}

	host := config.Host.ValueStringPointer()
	if host != nil {
		body.Data.Attributes.Host = host
	}

	if integrationID != nil {
		body.Data.Attributes.IntegrationId = integrationID
	}

	message := config.Message.ValueStringPointer()
	if message != nil {
		body.Data.Attributes.Message = message
	}

	if len(tags) != 0 {
		body.Data.Attributes.Tags = tags
	}

	if timestamp != nil {
		body.Data.Attributes.Timestamp = timestamp
	}

	evt, httpResp, err := a.Api.CreateEvent(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create Datadog event",
			fmt.Sprintf("Could not create event %s: %s", title, err),
		)
		return
	}

	eventUID := evt.GetData().Attributes.Attributes.Evt.GetUid()

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Created Datadog event %s (UID: %s) successfully (status: %d)", title, eventUID, httpResp.StatusCode),
	})

	tflog.Info(ctx, "Created Datadog event", map[string]any{
		"title": title,
		"uid":   eventUID,
	})
}
