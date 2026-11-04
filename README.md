# Terraform provider for Transmit on-call

Manage [Transmit](https://www.transmit.dev) on-call as code: responders, teams, schedules,
schedule overrides, escalation policies and heartbeats.

```hcl
terraform {
  required_providers {
    transmit = { source = "puchesjr/transmit" }
  }
}

provider "transmit" {} # reads TRANSMIT_API_KEY

resource "transmit_oncall_configuration" "prod" { name = "Production" }

resource "transmit_oncall_responder" "dana" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Dana"
  email            = "dana@example.com"
}

resource "transmit_oncall_schedule" "primary" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Primary"
  timezone         = "America/New_York"
  rotation {
    name          = "Weekly"
    cadence       = "weekly"
    starts_at     = "2026-11-02T09:00:00-05:00"
    responder_ids = [transmit_oncall_responder.dana.id]
  }
}

resource "transmit_oncall_escalation_policy" "default" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Default"
  step { schedule_id = transmit_oncall_schedule.primary.id }
}
```

Full reference: [docs/](docs/index.md). Examples: [examples/](examples/).

## Behaviour worth knowing

- **Every on-call change turns paging off** for that configuration and cancels pending escalation,
  exactly as a dashboard edit does. Run a drill and re-activate paging in the dashboard after an apply.
- Use a **live** API key with the `terraform` preset. Test keys cannot change on-call configuration.
- Deletes are refused while something still references the resource (a responder in a schedule, a
  schedule in a policy, a policy with heartbeats). Reference resources by id and Terraform orders it.
- An escalation policy that has paged incidents cannot be deleted; it is kept for their history.
- Destroying `transmit_oncall_configuration` only removes it from state.

## Development

```sh
go test ./...                                    # unit tests
TF_ACC=1 TRANSMIT_API_KEY=tr_v2_… \
  TRANSMIT_API_URL=http://127.0.0.1:3000/api \
  go test ./internal/provider -run TestAccOnCall # acceptance, against a running Transmit
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name transmit
```

Releases: push a `v*` tag. `.github/workflows/release.yml` builds and signs with GoReleaser; the
repository needs `GPG_PRIVATE_KEY` and `PASSPHRASE` secrets and the public key registered with the
Terraform Registry namespace.
