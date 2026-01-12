package api

import (
	"net/http"
	"sort"

	"github.com/clustercost/clustercost-dashboard/internal/finops"
)

// EfficiencyReport generates the FinOps efficiency analysis.
func (h *Handler) EfficiencyReport(w http.ResponseWriter, r *http.Request) {
	pods := h.store.GetAllPods()

	var reports []finops.EfficiencyReport
	ctx := r.Context()

	for _, pc := range pods {
		report, err := h.finops.CalculatePodEfficiency(ctx, pc.Pod, pc.ClusterID, pc.Region, pc.AZ, pc.InstanceType)
		if err != nil {
			// Log error but continue? Or skip?
			// For now continue and maybe return partial results?
			continue
		}
		if report != nil {
			reports = append(reports, *report)
		}
	}

	// Sort by Potential Savings Descending (highest waste first)
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].PotentialSavingsMo > reports[j].PotentialSavingsMo
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": reports,
		"count": len(reports),
	})
}
