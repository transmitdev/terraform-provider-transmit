resource "transmit_oncall_escalation_policy" "default" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Default"
  owner_team_id    = transmit_oncall_team.sre.id

  # Page whoever is on call now...
  step {
    schedule_id = transmit_oncall_schedule.primary.id
  }

  # ...then the team lead if nobody acknowledges within 10 minutes.
  step {
    delay_seconds = 600
    responder_id  = transmit_oncall_responder.lee.id
  }
}
