package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func importID(name string, attributes ...string) func(*terraform.State) (string, error) {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return "", fmt.Errorf("%s not in state", name)
		}
		parts := make([]string, len(attributes))
		for i, attribute := range attributes {
			parts[i] = rs.Primary.Attributes[attribute]
		}
		return strings.Join(parts, "/"), nil
	}
}
