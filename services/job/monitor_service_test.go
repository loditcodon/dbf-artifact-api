package job

import (
	"testing"
	"time"
)

// TestGetAllJobsPaginated_EmptyJobs tests pagination with no jobs
func TestGetAllJobsPaginated_EmptyJobs(t *testing.T) {
	jms := &JobMonitorService{
		jobs: make(map[string]*JobInfo),
	}

	result := jms.GetAllJobsPaginated(1, 10)

	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("Expected empty jobs array, got %d jobs", len(result.Jobs))
	}
	if result.Page != 1 {
		t.Errorf("Expected page 1, got %d", result.Page)
	}
	if result.PageSize != 10 {
		t.Errorf("Expected pageSize 10, got %d", result.PageSize)
	}
	if result.TotalPages != 0 {
		t.Errorf("Expected totalPages 0, got %d", result.TotalPages)
	}
}

// TestGetAllJobsPaginated_SinglePage tests pagination with jobs fitting in one page
func TestGetAllJobsPaginated_SinglePage(t *testing.T) {
	jms := &JobMonitorService{
		jobs: make(map[string]*JobInfo),
	}

	// Add 5 test jobs
	for i := 1; i <= 5; i++ {
		jms.jobs[string(rune(i))] = &JobInfo{
			JobID:     string(rune(i)),
			DBMgtID:   uint(i),
			Status:    "running",
			Progress:  0,
			StartTime: time.Now(),
		}
	}

	result := jms.GetAllJobsPaginated(1, 10)

	if result.Total != 5 {
		t.Errorf("Expected total 5, got %d", result.Total)
	}
	if len(result.Jobs) != 5 {
		t.Errorf("Expected 5 jobs, got %d", len(result.Jobs))
	}
	if result.TotalPages != 1 {
		t.Errorf("Expected 1 total page, got %d", result.TotalPages)
	}
}

// TestGetAllJobsPaginated_MultiplePages tests pagination across multiple pages
func TestGetAllJobsPaginated_MultiplePages(t *testing.T) {
	jms := &JobMonitorService{
		jobs: make(map[string]*JobInfo),
	}

	// Add 25 test jobs
	for i := 1; i <= 25; i++ {
		jobID := string(rune(i))
		jms.jobs[jobID] = &JobInfo{
			JobID:     jobID,
			DBMgtID:   uint(i),
			Status:    "running",
			Progress:  0,
			StartTime: time.Now(),
		}
	}

	// Test page 1
	result := jms.GetAllJobsPaginated(1, 10)
	if result.Total != 25 {
		t.Errorf("Expected total 25, got %d", result.Total)
	}
	if len(result.Jobs) != 10 {
		t.Errorf("Expected 10 jobs on page 1, got %d", len(result.Jobs))
	}
	if result.TotalPages != 3 {
		t.Errorf("Expected 3 total pages, got %d", result.TotalPages)
	}

	// Test page 2
	result = jms.GetAllJobsPaginated(2, 10)
	if len(result.Jobs) != 10 {
		t.Errorf("Expected 10 jobs on page 2, got %d", len(result.Jobs))
	}

	// Test page 3 (partial page)
	result = jms.GetAllJobsPaginated(3, 10)
	if len(result.Jobs) != 5 {
		t.Errorf("Expected 5 jobs on page 3, got %d", len(result.Jobs))
	}
}

// TestGetAllJobsPaginated_PageBeyondRange tests pagination beyond available data
func TestGetAllJobsPaginated_PageBeyondRange(t *testing.T) {
	jms := &JobMonitorService{
		jobs: make(map[string]*JobInfo),
	}

	// Add 5 test jobs
	for i := 1; i <= 5; i++ {
		jms.jobs[string(rune(i))] = &JobInfo{
			JobID:     string(rune(i)),
			DBMgtID:   uint(i),
			Status:    "running",
			Progress:  0,
			StartTime: time.Now(),
		}
	}

	// Request page 10 when only 1 page exists
	result := jms.GetAllJobsPaginated(10, 10)

	if len(result.Jobs) != 0 {
		t.Errorf("Expected empty jobs array for page beyond range, got %d jobs", len(result.Jobs))
	}
	if result.Total != 5 {
		t.Errorf("Expected total 5, got %d", result.Total)
	}
	if result.Page != 10 {
		t.Errorf("Expected requested page 10, got %d", result.Page)
	}
}

// TestGetAllJobsPaginated_InvalidParameters tests pagination with invalid parameters
func TestGetAllJobsPaginated_InvalidParameters(t *testing.T) {
	jms := &JobMonitorService{
		jobs: make(map[string]*JobInfo),
	}

	// Add test job
	jms.jobs["test"] = &JobInfo{
		JobID:     "test",
		DBMgtID:   1,
		Status:    "running",
		Progress:  0,
		StartTime: time.Now(),
	}

	tests := []struct {
		name             string
		page             int
		pageSize         int
		expectedPage     int
		expectedPageSize int
	}{
		{"Negative page", -1, 10, 1, 10},
		{"Zero page", 0, 10, 1, 10},
		{"Negative pageSize", 1, -1, 1, 10},
		{"Zero pageSize", 1, 0, 1, 10},
		{"Both invalid", -5, -5, 1, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jms.GetAllJobsPaginated(tt.page, tt.pageSize)

			if result.Page != tt.expectedPage {
				t.Errorf("%s: Expected page %d, got %d", tt.name, tt.expectedPage, result.Page)
			}
			if result.PageSize != tt.expectedPageSize {
				t.Errorf("%s: Expected pageSize %d, got %d", tt.name, tt.expectedPageSize, result.PageSize)
			}
		})
	}
}

// TestGetAllJobsPaginated_TotalPagesCalculation tests total pages calculation
func TestGetAllJobsPaginated_TotalPagesCalculation(t *testing.T) {
	tests := []struct {
		name               string
		totalJobs          int
		pageSize           int
		expectedTotalPages int
	}{
		{"Exact multiple", 20, 10, 2},
		{"With remainder", 25, 10, 3},
		{"Less than page size", 5, 10, 1},
		{"Empty", 0, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jms := &JobMonitorService{
				jobs: make(map[string]*JobInfo),
			}

			// Add jobs
			for i := 0; i < tt.totalJobs; i++ {
				jms.jobs[string(rune(i))] = &JobInfo{
					JobID:     string(rune(i)),
					DBMgtID:   uint(i),
					Status:    "running",
					Progress:  0,
					StartTime: time.Now(),
				}
			}

			result := jms.GetAllJobsPaginated(1, tt.pageSize)

			if result.TotalPages != tt.expectedTotalPages {
				t.Errorf("%s: Expected %d total pages, got %d", tt.name, tt.expectedTotalPages, result.TotalPages)
			}
		})
	}
}
