package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type validatorString = validator.String

var (
	uuidPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d{1,3})?)?(Z|[+-]\d{2}:\d{2})$`)
)

func nameValidators() []validator.String {
	return []validator.String{stringvalidator.LengthBetween(1, 120)}
}

func idValidators() []validator.String {
	return []validator.String{stringvalidator.RegexMatches(uuidPattern, "must be a lowercase UUID")}
}

func timestampValidators() []validator.String {
	return []validator.String{stringvalidator.RegexMatches(timestampPattern, "must be an RFC 3339 timestamp with an explicit offset, such as 2026-11-02T09:00:00-05:00")}
}
