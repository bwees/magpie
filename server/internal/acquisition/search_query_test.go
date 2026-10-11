package acquisition

import (
	"testing"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

func TestQueryPlanForSeriesEpisodes(t *testing.T) {
	tvdb := "371980"
	title := catalogapi.Title{Kind: domain.MediaKindSeries, Name: "Severance: Season", External: domain.ExternalIDs{Tmdb: 95396, Tvdb: &tvdb}}
	episode := catalogapi.Episode{Season: 2, Number: 3}
	full := TorznabCaps{TVSearch: true, TVParams: []TorznabParam{ParamQuery, ParamTVDB, ParamSeason, ParamEpisode}, Categories: []int{2000, 5000, 5040}}

	plan, hasID := queryPlan(full, searchTarget{Title: title, Episodes: []catalogapi.Episode{episode}})
	if !hasID || len(plan) != 2 {
		t.Fatalf("plan = %+v, want an ID tier and a title tier", plan)
	}
	byID, byTitle := plan[0], plan[1]
	if byID.Tvdb != tvdb || byID.Tmdb != 0 || *byID.Season != 2 || *byID.Episode != 3 {
		t.Errorf("ID query = %+v, want tvdbid with S02E03 params", byID)
	}
	if byTitle.Query != "Severance Season" || len(byTitle.Categories) != 2 {
		t.Errorf("title query = %+v", byTitle)
	}

	queryOnly := TorznabCaps{TVSearch: true, TVParams: []TorznabParam{ParamQuery}}
	plan, hasID = queryPlan(queryOnly, searchTarget{Title: title, Episodes: []catalogapi.Episode{episode}})
	if hasID || len(plan) != 1 || plan[0].Query != "Severance Season S02E03" || plan[0].Season != nil {
		t.Errorf("plan without params = %+v, want the episode in the query text", plan)
	}

	wholeSeason := []catalogapi.Episode{{Season: 1, Number: 1}, {Season: 1, Number: 2}}
	plan, _ = queryPlan(full, searchTarget{Title: title, Episodes: wholeSeason})
	if *plan[0].Season != 1 || plan[0].Episode != nil {
		t.Errorf("season query = %+v, want season 1 and no episode", plan[0])
	}
}
