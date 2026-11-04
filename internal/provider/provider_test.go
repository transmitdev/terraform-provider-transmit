package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"transmit": providerserver.NewProtocol6WithError(New("test")()),
}

func preCheck(t *testing.T) {
	if os.Getenv("TRANSMIT_API_KEY") == "" {
		t.Fatal("TRANSMIT_API_KEY must be set to a live key with the terraform preset; TRANSMIT_API_URL may point at a local server")
	}
}

func config(email, rotationName string, delay int, withOverride bool) string {
	override := ""
	if withOverride {
		override = `
resource "transmit_oncall_schedule_override" "vacation" {
  configuration_id = transmit_oncall_configuration.prod.id
  schedule_id      = transmit_oncall_schedule.primary.id
  starts_at        = "2026-12-24T00:00:00Z"
  ends_at          = "2026-12-27T00:00:00Z"
  responder_id     = transmit_oncall_responder.lee.id
  rotation_ids     = [transmit_oncall_schedule.primary.rotation_ids["` + rotationName + `"]]
}`
	}
	return fmt.Sprintf(`
resource "transmit_oncall_configuration" "prod" {
  name = "Terraform acceptance"
}

resource "transmit_oncall_responder" "dana" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Dana"
  email            = %[1]q
}

resource "transmit_oncall_responder" "lee" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Lee"
  email            = "lee-acc@example.com"
}

resource "transmit_oncall_team" "sre" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "SRE"
  responder_ids    = [transmit_oncall_responder.dana.id, transmit_oncall_responder.lee.id]
}

resource "transmit_oncall_schedule" "primary" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Primary"
  timezone         = "America/New_York"
  owner_team_id    = transmit_oncall_team.sre.id

  rotation {
    name          = %[2]q
    cadence       = "weekly"
    starts_at     = "2026-11-02T09:00:00-05:00"
    responder_ids = [transmit_oncall_responder.dana.id, transmit_oncall_responder.lee.id]
  }
}
%[4]s
resource "transmit_oncall_escalation_policy" "default" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Default"

  step {
    schedule_id = transmit_oncall_schedule.primary.id
  }
  step {
    delay_seconds = %[3]d
    responder_id  = transmit_oncall_responder.lee.id
  }
}

resource "transmit_heartbeat" "backup" {
  name                 = "tf-acc-nightly-backup"
  configuration_id     = transmit_oncall_configuration.prod.id
  escalation_policy_id = transmit_oncall_escalation_policy.default.id
  interval_seconds     = 86400
}

data "transmit_oncall_responder" "dana" {
  configuration_id = transmit_oncall_configuration.prod.id
  email            = transmit_oncall_responder.dana.email
}
`, email, rotationName, delay, override)
}

func TestAccOnCall(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config("dana-acc@example.com", "Weekly", 600, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("transmit_oncall_configuration.prod", "paging_enabled", "false"),
					resource.TestCheckResourceAttr("transmit_oncall_responder.dana", "disabled", "false"),
					resource.TestCheckResourceAttr("transmit_oncall_team.sre", "responder_ids.#", "2"),
					resource.TestCheckResourceAttr("transmit_oncall_schedule.primary", "rotation.0.interval", "1"),
					resource.TestCheckResourceAttrSet("transmit_oncall_schedule.primary", "rotation_ids.Weekly"),
					resource.TestCheckResourceAttrPair("transmit_oncall_schedule_override.vacation", "rotation_ids.0", "transmit_oncall_schedule.primary", "rotation_ids.Weekly"),
					resource.TestCheckResourceAttrPair("transmit_oncall_escalation_policy.default", "step.0.schedule_id", "transmit_oncall_schedule.primary", "id"),
					resource.TestCheckResourceAttr("transmit_oncall_escalation_policy.default", "step.0.delay_seconds", "0"),
					resource.TestCheckResourceAttr("transmit_heartbeat.backup", "grace_seconds", "300"),
					resource.TestCheckResourceAttrSet("transmit_heartbeat.backup", "ping_url"),
					resource.TestCheckResourceAttrPair("data.transmit_oncall_responder.dana", "id", "transmit_oncall_responder.dana", "id"),
				),
			},
			// In-place updates: no resource is replaced, and the override survives a schedule edit.
			{
				Config: config("dana-new@example.com", "Weekly", 900, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("transmit_oncall_responder.dana", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("transmit_oncall_escalation_policy.default", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("transmit_oncall_schedule_override.vacation", plancheck.ResourceActionNoop),
				}},
				Check: resource.TestCheckResourceAttr("transmit_oncall_escalation_policy.default", "step.1.delay_seconds", "900"),
			},
			{ResourceName: "transmit_oncall_responder.dana", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: importID("transmit_oncall_responder.dana", "configuration_id", "id")},
			{ResourceName: "transmit_oncall_schedule.primary", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: importID("transmit_oncall_schedule.primary", "configuration_id", "id")},
			{ResourceName: "transmit_oncall_escalation_policy.default", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: importID("transmit_oncall_escalation_policy.default", "configuration_id", "id")},
			{ResourceName: "transmit_oncall_schedule_override.vacation", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: importID("transmit_oncall_schedule_override.vacation", "configuration_id", "schedule_id", "id")},
			{ResourceName: "transmit_heartbeat.backup", ImportState: true, ImportStateVerify: true},
			// Dropping the override and renaming the rotation in one apply: Terraform deletes the override first.
			{Config: config("dana-new@example.com", "Follow the sun", 900, false), Check: resource.TestCheckResourceAttrSet("transmit_oncall_schedule.primary", "rotation_ids.Follow the sun")},
		},
	})
}
