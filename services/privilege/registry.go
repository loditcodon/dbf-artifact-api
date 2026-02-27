package privilege

import (
	"fmt"
	"sync"

	"dbfartifactapi/models"
)

// registry holds function references registered by services/ to break circular dependency.
// Functions are registered at init time before any handler code executes.
var (
	registryMu               sync.RWMutex
	newPolicyEvaluatorFn     NewPolicyEvaluatorFunc
	retrieveJobResultsFn     RetrieveJobResultsFunc
	getEndpointForJobFn      GetEndpointForJobFunc
)

// RegisterNewPolicyEvaluator registers factory for creating PolicyEvaluator instances.
// Must be called by services/ package init before any handler executes.
func RegisterNewPolicyEvaluator(fn NewPolicyEvaluatorFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	newPolicyEvaluatorFn = fn
}

// RegisterRetrieveJobResults registers function for fetching job results from VeloArtifact.
// Must be called by services/ package init before any handler executes.
func RegisterRetrieveJobResults(fn RetrieveJobResultsFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	retrieveJobResultsFn = fn
}

// RegisterGetEndpointForJob registers function for retrieving endpoint by ID.
// Must be called by services/ package init before any handler executes.
func RegisterGetEndpointForJob(fn GetEndpointForJobFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	getEndpointForJobFn = fn
}

// NewPolicyEvaluator creates a new PolicyEvaluator via registered factory.
func NewPolicyEvaluator() PolicyEvaluator {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if newPolicyEvaluatorFn == nil {
		panic("privilege: NewPolicyEvaluator not registered - services/ init must call RegisterNewPolicyEvaluator")
	}
	return newPolicyEvaluatorFn()
}

// RetrieveJobResults fetches job results via registered function.
func RetrieveJobResults(jobID string, ep *models.Endpoint) ([]QueryResult, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if retrieveJobResultsFn == nil {
		return nil, fmt.Errorf("privilege: RetrieveJobResults not registered")
	}
	return retrieveJobResultsFn(jobID, ep)
}

// GetEndpointForJob retrieves endpoint via registered function.
func GetEndpointForJob(jobID string, endpointID uint) (*models.Endpoint, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if getEndpointForJobFn == nil {
		return nil, fmt.Errorf("privilege: GetEndpointForJob not registered")
	}
	return getEndpointForJobFn(jobID, endpointID)
}
