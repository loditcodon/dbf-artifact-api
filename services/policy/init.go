package policy

import (
	"dbfartifactapi/models"
	"dbfartifactapi/services/privilege"
)

func init() {
	// Register policy package dependencies with privilege registry to break circular import.
	// privilege -> policy would create a cycle, so privilege defines interfaces and
	// policy registers concrete implementations at init time.

	privilege.RegisterNewPolicyEvaluator(func() privilege.PolicyEvaluator {
		return NewDBPolicyService().(*dbPolicyService)
	})

	privilege.RegisterRetrieveJobResults(func(jobID string, ep *models.Endpoint) ([]privilege.QueryResult, error) {
		results, err := RetrieveJobResults(jobID, ep)
		if err != nil {
			return nil, err
		}
		// Convert policy.QueryResult to privilege.QueryResult
		out := make([]privilege.QueryResult, len(results))
		for i, r := range results {
			out[i] = privilege.QueryResult{
				QueryKey:    r.QueryKey,
				Query:       r.Query,
				Status:      r.Status,
				Result:      r.Result,
				ExecuteTime: r.ExecuteTime,
				DurationMs:  r.DurationMs,
			}
		}
		return out, nil
	})

	privilege.RegisterGetEndpointForJob(GetEndpointForJob)
}
