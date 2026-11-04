resource "transmit_oncall_team" "sre" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "SRE"
  responder_ids    = [transmit_oncall_responder.dana.id, transmit_oncall_responder.lee.id]
}
