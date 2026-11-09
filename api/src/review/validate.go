package review

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limits. They apply to what a caller sends; the database repeats the ones that matter as CHECKs so a bug elsewhere cannot store
// something out of range.
const (
	MaxBodyRunes       = 2000
	MaxEntityTypeRunes = 40
	MaxSummaryIDs      = 100
	maxAuthorIDRunes   = 200
	maxAuthorNameRunes = 100
	maxOwnerRunes      = 64
	DefaultPageSize    = 50
	MaxPageSize        = 100
)

// Scales are the three star scales a type can use.
var Scales = []int{3, 5, 10}

// SystemOwner is the owner name of Cinqo itself. A tool with this slug would be indistinguishable from the system, so the engine
// always treats it as the system.
const SystemOwner = "cinqo"

var (
	// ErrNotFound: no such review (for that entity).
	ErrNotFound = errors.New("review not found")
	// ErrInvalidOwner: the owner name cannot name a reviews file.
	ErrInvalidOwner = errors.New("invalid review owner")

	namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// ValidationError is a caller mistake: which field and why.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// IsValidation reports whether err is a *ValidationError.
func IsValidation(err error) (*ValidationError, bool) {
	var v *ValidationError
	return v, errors.As(err, &v)
}

// ValidScale reports whether n is one of the three star scales.
func ValidScale(n int) bool { return n == 3 || n == 5 || n == 10 }

// halfSteps turns a rating in stars into a whole number of half-stars, or says why it cannot be one. A type without half stars
// accepts whole stars only; every rating is at least half a star (one star without halves) and at most the scale. The rating is
// never stored as a float, so 3.5 is 7 and 3.25 is refused, not rounded.
func halfSteps(rating float64, scale int, halfStars bool) (int, error) {
	if !ValidScale(scale) {
		return 0, invalid("scale", "must be 3, 5 or 10")
	}
	if math.IsNaN(rating) || math.IsInf(rating, 0) {
		return 0, invalid("rating", "must be a number")
	}
	doubled := rating * 2
	steps := math.Round(doubled)
	if math.Abs(doubled-steps) > 1e-9 {
		if halfStars {
			return 0, invalid("rating", "must be a whole or a half number of stars")
		}
		return 0, invalid("rating", "must be a whole number of stars")
	}
	n := int(steps)
	if !halfStars && n%2 != 0 {
		return 0, invalid("rating", "must be a whole number of stars")
	}
	if n < 1 || n > scale*2 {
		return 0, invalid("rating", fmt.Sprintf("must be between %s and %d stars", minimum(halfStars), scale))
	}
	return n, nil
}

func minimum(halfStars bool) string {
	if halfStars {
		return "0.5"
	}
	return "1"
}

// cleanBody trims the optional text and checks it: not over the limit, no NUL. Empty is allowed (a rating alone).
func cleanBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if strings.ContainsRune(body, 0) {
		return "", invalid("body", "contains an invalid character")
	}
	if n := utf8.RuneCountInString(body); n > MaxBodyRunes {
		return "", invalid("body", fmt.Sprintf("must be at most %d characters", MaxBodyRunes))
	}
	return body, nil
}

// cleanEntityType checks an entity type's shape (whether an owner registered it is the policy layer's question, not storage's).
func cleanEntityType(t string) (string, error) {
	if utf8.RuneCountInString(t) > MaxEntityTypeRunes || !namePattern.MatchString(t) {
		return "", invalid("entityType", "must start with a lower-case letter and contain only lower-case letters, digits and underscores (at most 40)")
	}
	return t, nil
}

// ValidEntityType reports whether t is an acceptable entity type name; manifests use it so an install is refused for a type the
// engine would later refuse to store.
func ValidEntityType(t string) bool {
	_, err := cleanEntityType(t)
	return err == nil
}

// canonicalUUID accepts a UUID in its usual 36-character form, in either case, and returns it lower-cased. Braces, "urn:uuid:" and
// hyphen-less forms are refused so one entity can never have two spellings.
func canonicalUUID(field, s string) (string, error) {
	if len(s) != 36 {
		return "", invalid(field, "must be a UUID")
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return "", invalid(field, "must be a UUID")
	}
	return id.String(), nil
}

// cleanAuthor trims the author fields. The id is required; an over-long display name is shortened rather than refused (it comes
// from a token, not from the user typing).
func cleanAuthor(id, name string) (string, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", invalid("authorId", "is required")
	}
	if utf8.RuneCountInString(id) > maxAuthorIDRunes || strings.ContainsRune(id, 0) {
		return "", "", invalid("authorId", "is not valid")
	}
	name = strings.TrimSpace(strings.ReplaceAll(name, "\x00", ""))
	if r := []rune(name); len(r) > maxAuthorNameRunes {
		name = string(r[:maxAuthorNameRunes])
	}
	return id, name, nil
}

// checkOwner: the system, or a tool slug. The slug pattern excludes every path separator, dot and space, so an accepted owner can
// only ever name one directory entry directly under db/tools.
func checkOwner(owner string) error {
	if owner == SystemOwner {
		return nil
	}
	if utf8.RuneCountInString(owner) > maxOwnerRunes || !namePattern.MatchString(owner) {
		return ErrInvalidOwner
	}
	return nil
}
