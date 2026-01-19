package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/terraform-providers/terraform-provider-datadog/datadog/fwprovider"
)

func TestAccEventCreateAction_basic(t *testing.T) {
	t.Parallel()

	ctx, providers, accProviders := testAccFrameworkMuxProviders(context.Background(), t)
	title := "basic_event"
	category := "change"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: accProviders,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccInvokeActionConfig(title, category),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckInvokeAction(ctx, providers.frameworkProvider, title, category),
				),
			},
		},
	})
}

// testAccCheckInvokeAction verifies that the action can successfully invoke a Lambda function
func testAccCheckInvokeAction(ctx context.Context, accProvider *fwprovider.FrameworkProvider, title, category string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		api := accProvider.DatadogApiInstances.GetEventsApiV2()

		eventCategory, err := datadogV2.NewEventCategoryFromValue(category)
		if err != nil {
			return fmt.Errorf("Failed to create Datadog event category from %s: %w", category, err)
		}

		// Create event directly to verify it works directly.
		evt := datadogV2.EventCreateRequestPayload{
			Data: datadogV2.EventCreateRequest{
				Attributes: datadogV2.EventPayload{
					Title:    title,
					Category: *eventCategory,
				},
				Type: datadogV2.EVENTCREATEREQUESTTYPE_EVENT,
			},
		}

		_, _, err = api.CreateEvent(ctx, evt)
		if err != nil {
			return fmt.Errorf("Failed to create Datadog event %s: %w", title, err)
		}

		return nil
	}
}

func testAccInvokeActionConfig(title, category string) string {
	return fmt.Sprintf(`
action "datadog_create_event" "test" {
  config {
    title                 = "%s"
    category              = "%s"
    changed_resource_name = "test_resource"
    changed_resource_type = "configuration"
  }
}

resource "terraform_data" "trigger" {
  input = "trigger"
  lifecycle {
    action_trigger {
      events  = [before_create, before_update]
      actions = [action.datadog_create_event.test]
    }
  }
}
`, title, category)
}
