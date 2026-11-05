package render

import "strings"

// rules checks across snippets after the merge (SPEC 6.4). Each finding
// has an id, a level (error stops a render; warn and hint don't) and a fix.
// More come with the snippets they're about (PLAN M1, M2).
func rules(uses []*use, base *use, app App, res *Result) []Finding {
	var out []Finding
	if !fullImageName(app.Image) {
		out = append(out, Finding{
			Rule:  "image-full-name",
			Level: "error",
			Msg:   "base image " + app.Image + " is a short name: podman without search registries fails with \"short-name did not resolve\"",
			Fix:   "write the registry too, e.g. docker.io/library/" + app.Image,
		})
	}
	return out
}

// fullImageName: the first part of the name is a registry host.
func fullImageName(img string) bool {
	first, _, ok := strings.Cut(img, "/")
	return ok && (strings.ContainsAny(first, ".:") || first == "localhost")
}
