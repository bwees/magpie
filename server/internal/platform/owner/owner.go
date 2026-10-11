// Package owner names the platform and the contexts that own tables, migrations and settings.
package owner

import "slices"

type Name string

const (
	Platform    Name = "platform"
	Identity    Name = "identity"
	Catalog     Name = "catalog"
	Requests    Name = "requests"
	Library     Name = "library"
	Acquisition Name = "acquisition"
	Subtitles   Name = "subtitles"
	Transcode   Name = "transcode"
	Activity    Name = "activity"
)

// All lists every owner in migration order: the platform first, then contexts in query-dependency order.
var All = []Name{Platform, Identity, Catalog, Requests, Library, Acquisition, Subtitles, Transcode, Activity}

func (n Name) Known() bool {
	return slices.Contains(All, n)
}
