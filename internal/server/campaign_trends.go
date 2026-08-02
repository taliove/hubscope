package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// trendModelDTO is the model identity block of the trends response. Deleted
// covers both ticket-26 senses (manually deleted or hub-retired); the trend
// stays readable either way, badged in the UI.
type trendModelDTO struct {
	ModelDBID int64  `json:"model_db_id"`
	ModelID   string `json:"model_id"`
	Family    string `json:"family"`
	Deleted   bool   `json:"deleted"`
}

// trendPointDTO is one campaign on a model's score trend: the 0-100 score
// (null when the batch judged nothing), the suite version it scored against,
// and whether the question bank changed vs the previous point (ADR 0007 —
// the two sides of a break are not comparable). VerdictProfile names the
// scoring caliber of the point and ProfileChanged flags a caliber break vs
// the previous point (ADR 0008) — the same break semantics as a suite
// version change: scores across it are not comparable.
type trendPointDTO struct {
	CampaignID     int64    `json:"campaign_id"`
	Score          *float64 `json:"score"`
	SuiteVersion   int      `json:"suite_version"`
	VersionChanged bool     `json:"version_changed"`
	VerdictProfile string   `json:"verdict_profile"`
	ProfileChanged bool     `json:"profile_changed"`
}

// trendSuiteDTO is one suite's cross-campaign score series, ordered by
// campaign.
type trendSuiteDTO struct {
	SuiteID int64           `json:"suite_id"`
	Key     string          `json:"key"`
	Name    string          `json:"name"`
	Points  []trendPointDTO `json:"points"`
}

// campaignTrendsDTO is GET /api/campaigns/{id}/trends: the model's identity,
// its per-suite score trend across settled campaigns up to and including
// this one, and the probe-side hourly aggregate over the same timeline.
type campaignTrendsDTO struct {
	Model  trendModelDTO     `json:"model"`
	Suites []trendSuiteDTO   `json:"suites"`
	Probe  []seriesBucketDTO `json:"probe"`
}

// handleGetCampaignTrends handles GET /api/campaigns/{id}/trends?model=<dbID>.
// The trend is fetched on demand per model (the report page's drill-down),
// so the model is a required query parameter.
func (s *Server) handleGetCampaignTrends(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}
	modelDBID, err := parseTrendModelParam(r.URL.Query().Get("model"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	identity, found, err := s.db.GetModelIdentity(modelDBID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve model")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	points, err := s.db.ListModelTrend(modelDBID, campaign.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load score trend")
		return
	}

	// The probe side spans the same timeline as the score trend: from the
	// earliest settled campaign (or this campaign's own start when nothing
	// has settled yet) to this campaign's end (now when still running).
	since, ok, err := s.db.EarliestSettledCampaignStart(campaign.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to bound trend window")
		return
	}
	if !ok {
		if campaign.StartedAt != nil {
			since = *campaign.StartedAt
		} else {
			since = s.now().UTC()
		}
	}
	until := s.now().UTC()
	if campaign.FinishedAt != nil {
		until = *campaign.FinishedAt
	}
	probe, err := s.db.GetModelProbeSeries(modelDBID, since, until)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load probe trend")
		return
	}

	writeData(w, http.StatusOK, campaignTrendsDTO{
		Model: trendModelDTO{
			ModelDBID: modelDBID,
			ModelID:   identity.ModelID,
			Family:    identity.Family,
			Deleted:   identity.Deleted,
		},
		Suites: buildTrendSuites(points),
		Probe:  toSeriesBucketDTOs(probe),
	})
}

// parseTrendModelParam validates the required model query parameter: a
// positive integer database id.
func parseTrendModelParam(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if raw == "" || err != nil || id <= 0 {
		return 0, fmt.Errorf("model must be a positive integer model id")
	}
	return id, nil
}

// buildTrendSuites groups flat (campaign, suite) points into per-suite
// series, scaling raw means through the ADR-0009 nadir normalization (each
// point with its own run's snapshot constant) and flagging the point where
// the suite version or the verdict profile changes. Points arrive ordered
// by campaign, so a simple consecutive comparison detects both kinds of
// break.
func buildTrendSuites(points []store.TrendPoint) []trendSuiteDTO {
	suites := []trendSuiteDTO{}
	indexBySuite := map[int64]int{}
	for _, p := range points {
		idx, ok := indexBySuite[p.SuiteID]
		if !ok {
			idx = len(suites)
			indexBySuite[p.SuiteID] = idx
			suites = append(suites, trendSuiteDTO{
				SuiteID: p.SuiteID,
				Key:     p.SuiteKey,
				Name:    p.SuiteName,
				Points:  []trendPointDTO{},
			})
		}
		var scaled *float64
		if p.Score != nil {
			v := normalizeScore(*p.Score, p.Nadir)
			scaled = &v
		}
		prev := suites[idx].Points
		changed := len(prev) > 0 && prev[len(prev)-1].SuiteVersion != p.SuiteVersion
		profileChanged := len(prev) > 0 && prev[len(prev)-1].VerdictProfile != p.VerdictProfile
		suites[idx].Points = append(suites[idx].Points, trendPointDTO{
			CampaignID:     p.CampaignID,
			Score:          scaled,
			SuiteVersion:   p.SuiteVersion,
			VersionChanged: changed,
			VerdictProfile: p.VerdictProfile,
			ProfileChanged: profileChanged,
		})
	}
	return suites
}

// toSeriesBucketDTOs converts store-level probe buckets into the shared
// series DTO shape (same field names as the endpoint series API).
func toSeriesBucketDTOs(buckets []store.ModelProbeBucket) []seriesBucketDTO {
	dtos := make([]seriesBucketDTO, 0, len(buckets))
	for _, b := range buckets {
		dtos = append(dtos, seriesBucketDTO{
			BucketStart: b.BucketStart.UTC().Format(time.RFC3339),
			Total:       b.Total,
			Failures:    b.Failures,
			P50Ms:       b.P50Ms,
			P95Ms:       b.P95Ms,
		})
	}
	return dtos
}
