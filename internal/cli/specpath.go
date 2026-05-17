package cli

import "lingua/helm"

func ResolveHelmLSpecPath() (string, error) {
	return helm.ResolveHelmLSpecPath()
}
