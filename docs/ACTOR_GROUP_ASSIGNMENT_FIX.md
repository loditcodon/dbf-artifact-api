# Actor-to-Group Assignment Bug Fix

## Bug Report

### Endpoint
`GET /api/queries/dbpolicy/cntmgt/{id}` → `GetByCntMgt`

### Symptom
All actors are assigned to **group 1** regardless of their actual policy profiles.

```
[INFO] Assigned actor 4 to group 1
[INFO] Assigned actor 11 to group 1
[INFO] Assigned actor 5 to group 1
[INFO] Assigned actor 10 to group 1
[INFO] Assigned actor 7 to group 1
[INFO] Assigned actor 13 to group 1
[INFO] Assigned actor 14 to group 1
[INFO] Assigned actor 15 to group 1
[INFO] Assigned actor 2 to group 1
[INFO] Assigned actor 8 to group 1
[INFO] Assigned actor 3 to group 1
```

### Root Cause

**File:** `services/privilege_session_handler.go`, function `assignActorsToGroups()`

The original algorithm had two flaws:

#### Flaw 1: Single-level "ANY Match" logic

A single overlapping policy between an actor and a `dbgroup_listpolicies` entry was enough to consider it a match. Then any group linked to that listpolicy became a candidate. This skipped the requirement that an actor must satisfy ALL listpolicies of a group.

#### Flaw 2: "Lowest ID Wins" without full validation

Among all candidate groups, the algorithm always picked the one with the lowest ID. Since group 1 had all 29 listpolicies linked to it and ANY single policy overlap counted as a match, group 1 always won.

#### Flaw 3: Missing two-level matching

The correct business logic requires **two levels** of matching:
1. Actor must satisfy a listpolicy (have ALL its `dbpolicydefault_id` values)
2. Actor must satisfy ALL listpolicies linked to a group (via `dbpolicy_groups`)

The original code only did a loose version of Level 1 and completely skipped Level 2.

---

## Fix: Two-Level Strict Exact Match Strategy

### Strategy

Replace "any match + lowest ID" with "two-level 100% exact match":

1. **Level 1:** For each `dbgroup_listpolicies`, check if actor has ALL policies in `dbpolicydefault_id`
2. **Level 2:** For each group, check if actor satisfies ALL `dbgroup_listpolicies` linked to that group
3. Among fully-matched groups, select the one with the lowest group ID
4. If no group is fully matched, skip the actor

### Two-Level Matching Definition

#### Level 1: Actor vs ListPolicy (actor ⊇ listpolicy.dbpolicydefault_id)

```
dbgroup_listpolicies ID=10 (Select): dbpolicydefault_id = {7, 8, 9}
actor policies = {3, 4, 7, 8, 9, 10, 11, 12}

Does actor have ALL of {7, 8, 9}?  → YES  → actor satisfies listpolicy 10 ✅
```

```
dbgroup_listpolicies ID=1 (Super): dbpolicydefault_id = {93, 94, 96, 140, 144, ...}
actor policies = {3, 4, 7, 8, 9, 10, 11, 12}

Does actor have ALL of {93, 94, 96, 140, ...}?  → NO  → actor does NOT satisfy listpolicy 1 ❌
```

#### Level 2: Actor's satisfied listpolicies vs Group requirements

```
Group 1 requires listpolicies: [1, 2, 3, 4, 5, 6, ..., 29]  (29 total, from dbpolicy_groups)
Actor satisfies listpolicies:  [4, 5, 6, 8, 9, 10, 11, 12, 13, 16, 17, 18, 19, 20, 21, 22, 24, 25, 26, 27, 28]  (21 of 29)

Does actor satisfy ALL 29 required listpolicies?  → NO (missing 1,2,3,7,14,15,23,29)  → NOT assigned to Group 1 ❌
```

```
Group 2 requires listpolicies: [5, 6, 10, 11, 12, 13]  (6 total)
Actor satisfies listpolicies:  [..., 5, 6, ..., 10, 11, 12, 13, ...]

Does actor satisfy ALL 6 required listpolicies?  → YES  → assigned to Group 2 ✅
```

### Data Model Relationships

```
dbgroup_listpolicies (defines policy lists)
  ├── ID=1 "Super"         → dbpolicydefault_id: {93,94,96,140,...}
  ├── ID=10 "Select"       → dbpolicydefault_id: {7,8,9}
  └── ID=11 "Insert"       → dbpolicydefault_id: {10,11,12}

dbpolicy_groups (links listpolicies to groups)
  ├── listpolicy_id=1  → group_id=1
  ├── listpolicy_id=10 → group_id=1
  ├── listpolicy_id=10 → group_id=2    ← same listpolicy can link to multiple groups
  └── listpolicy_id=11 → group_id=2

dbgroupmgt (group definitions)
  ├── ID=1 "Full Admin"    ← requires ALL 29 listpolicies
  └── ID=2 "Read-Write"    ← requires only Select + Insert
```

### Implementation Details

#### Helper functions (Level 1)

```go
// isExactMatch checks if actor has ALL policies that a listpolicy requires (group ⊆ actor).
func isExactMatch(actorPolicies map[uint]bool, groupPolicies map[uint]bool) bool

// collectExactMatchGroups finds all listpolicies where actor has every required policy.
func collectExactMatchGroups(actorPolicies map[uint]bool, groupListPolicies []groupListPolicy) []uint

// getPolicyIDsAsSlice converts policy ID map to sorted slice for logging.
func getPolicyIDsAsSlice(policyMap map[uint]bool) []uint
```

#### Main algorithm in `assignActorsToGroups()`

```
1. Load dbgroup_listpolicies → parse dbpolicydefault_id for each
2. Load dbpolicy_groups → build map: group_id → set of required listpolicy_ids
3. For each actor:
   a. Level 1: Find which listpolicies the actor fully satisfies
      satisfiedListPolicies = collectExactMatchGroups(actorPolicies, allListPolicies)
   b. Level 2: Find groups where actor satisfies ALL required listpolicies
      For each group:
        If group.requiredListPolicies ⊆ actor.satisfiedListPolicies → candidate
   c. Select candidate group with lowest ID
   d. Create DBActorGroups assignment
```

### Test Scenarios

| # | Actor Satisfied ListPolicies | Group 1 Requires | Group 2 Requires | Expected |
|---|------------------------------|-------------------|-------------------|----------|
| 1 | `[1,2,3,...,29]` (all 29) | `[1,...,29]` | `[10,11]` | Group 1 (satisfies both, ID 1 < 2) |
| 2 | `[5,6,10,11,12,13]` | `[1,...,29]` | `[10,11,12,13]` | Group 2 (only satisfies Group 2) |
| 3 | `[10,11]` | `[1,...,29]` | `[10,11,12,13]` | Skip (missing listpolicies for both) |
| 4 | `[]` | any | any | Skip (no policies) |
| 5 | `[10,11,12,13,16]` | `[10,11,12,13]` (inactive) | `[10,11,12,13,16]` (active) | Group 2 (Group 1 filtered by is_active) |

### Unit Tests

```go
// Level 1 tests
func TestIsExactMatch_ActorHasAllGroupPolicies_ReturnsTrue(t *testing.T)
func TestIsExactMatch_ActorSupersetOfGroup_ReturnsTrue(t *testing.T)
func TestIsExactMatch_ActorMissingGroupPolicy_ReturnsFalse(t *testing.T)
func TestIsExactMatch_ActorSubsetOfGroup_ReturnsFalse(t *testing.T)
func TestIsExactMatch_EmptyActor_ReturnsFalse(t *testing.T)
func TestIsExactMatch_EmptyGroup_ReturnsFalse(t *testing.T)
func TestIsExactMatch_IdenticalSets_ReturnsTrue(t *testing.T)
func TestCollectExactMatchGroups_MultipleMatches_ReturnsAll(t *testing.T)
func TestCollectExactMatchGroups_NoMatch_ReturnsEmpty(t *testing.T)
func TestGetPolicyIDsAsSlice_ReturnsSortedSlice(t *testing.T)
```

### Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Many actors skipped (no exact match) | Actors left without group | Review `dbgroup_listpolicies` data coverage before deploy |
| Performance regression | Slower assignment | `isExactMatch` exits early on first mismatch; group requirements loaded once |
| Backward incompatibility | Existing assignments differ | No schema change; re-run will reassign correctly |

### Rollback Plan

1. Revert the commit
2. Clean up new assignments:
   ```sql
   DELETE FROM dbactor_groups
   WHERE created_at >= '<deploy_timestamp>'
   AND is_active = 1;
   ```
3. Re-run old logic

### Pre-Deploy Checklist

- [ ] Review `dbgroup_listpolicies` data to estimate skip rate
- [ ] Ensure `dbpolicy_groups` has different groups with different listpolicy sets
- [ ] Backup `dbactor_groups` table
- [ ] Run unit tests
- [ ] Test with real data on staging
- [ ] Compare old vs new assignment results
- [ ] Deploy and monitor logs for skip warnings
