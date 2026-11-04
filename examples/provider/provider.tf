terraform {
  required_providers {
    transmit = {
      source = "puchesjr/transmit"
    }
  }
}

# Create a live key in Settings → API keys with the "terraform" scope preset,
# then export TRANSMIT_API_KEY=tr_v2_...
provider "transmit" {}
