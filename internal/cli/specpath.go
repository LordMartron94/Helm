package cli

import "lingua/helm"

func ResolveHelmLSpecPath() (path string, release func(), err error) {
	return helm.ResolveHelmLSpecPath()
}
