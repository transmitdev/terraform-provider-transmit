resource "transmit_oncall_responder" "dana" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Dana Reyes"
  email            = "dana@example.com"
}
