data "transmit_oncall_responder" "dana" {
  configuration_id = data.transmit_oncall_configuration.imported.id
  email            = "dana@example.com"
}
