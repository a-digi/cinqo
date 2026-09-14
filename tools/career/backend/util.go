// util.go holds small helpers shared by profile.go and jobs.go.
package main

import "encoding/json"

func jsonMarshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
