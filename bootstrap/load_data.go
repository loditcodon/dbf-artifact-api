package bootstrap

import (
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"fmt"
)

// Package-level variables store cached bootstrap data for quick lookup throughout the application.
var (
	// DBActorAll stores all database actors loaded at startup.
	DBActorAll []models.DBActor
	// DBTypeAll stores all database types loaded at startup.
	DBTypeAll []models.DBType
	// DBObjectAllMap stores database objects indexed by ID for quick lookup.
	DBObjectAllMap map[uint]models.DBObject
	// DBPolicyDefaultsAllMap stores policy templates indexed by ID for quick lookup.
	DBPolicyDefaultsAllMap map[uint]models.DBPolicyDefault
	// DBGroupListPoliciesAllMap stores policy group mappings indexed by ID for quick lookup.
	DBGroupListPoliciesAllMap map[uint]models.DBGroupListPolicies
)

// LoadData initializes bootstrap data including policy defaults, actors, and object mappings.
func LoadData() error {
	logger.Infof("Starting bootstrap data loading...")

	actorRepo := repository.NewDBActorRepository()
	dbTypeRepo := repository.NewDBTypeRepository()
	dbObjectRepo := repository.NewDBObjectRepository()
	dbPolicyDefaultRepo := repository.NewDBPolicyDefaultRepository()
	dbGroupListPoliciesRepo := repository.NewDBGroupListPoliciesRepository()

	if err := loadActorAll(actorRepo); err != nil {
		return err
	}
	if err := loadDBTypeAll(dbTypeRepo); err != nil {
		return err
	}
	if err := loadDBObjectAll(dbObjectRepo); err != nil {
		return err
	}
	if err := loadDBPolicyDefaultAll(dbPolicyDefaultRepo); err != nil {
		return err
	}
	if err := loadDBGroupListPoliciesAll(dbGroupListPoliciesRepo); err != nil {
		return err
	}

	logger.Infof("Bootstrap data loading completed successfully")
	return nil
}

func loadActorAll(repo repository.DBActorRepository) error {
	actors, err := repo.GetAll(nil)
	if err != nil {
		logger.Errorf("Failed to load all actors: %v", err)
		return fmt.Errorf("failed to load all actors: %v", err)
	}
	DBActorAll = actors
	logger.Infof("Loaded %d actors", len(actors))
	return nil
}

func loadDBTypeAll(repo repository.DBTypeRepository) error {
	dbtypes, err := repo.GetAll(nil)
	if err != nil {
		logger.Errorf("Failed to load all dbtypes: %v", err)
		return fmt.Errorf("failed to load all dbtypes: %v", err)
	}
	DBTypeAll = dbtypes
	logger.Infof("Loaded %d dbtypes", len(dbtypes))
	return nil
}

func loadDBObjectAll(repo repository.DBObjectRepository) error {
	dbObjects, err := repo.GetAll()
	if err != nil {
		logger.Errorf("Failed to load all dbobjects: %v", err)
		return fmt.Errorf("failed to load all dbobjects: %v", err)
	}
	dbObjectMap := make(map[uint]models.DBObject)
	for _, dbObject := range dbObjects {
		dbObjectMap[dbObject.ID] = dbObject
	}
	DBObjectAllMap = dbObjectMap
	logger.Infof("Loaded %d dbobjects", len(dbObjects))
	return nil
}

func loadDBPolicyDefaultAll(repo repository.DBPolicyDefaultRepository) error {
	dbPolicyDefaults, err := repo.GetAll(nil)
	if err != nil {
		logger.Errorf("Failed to load all dbpolicydefaults: %v", err)
		return fmt.Errorf("failed to load all dbpolicydefaults: %v", err)
	}
	dbPolicyDefaultMap := make(map[uint]models.DBPolicyDefault)
	for _, dbPD := range dbPolicyDefaults {
		dbPolicyDefaultMap[dbPD.ID] = dbPD
	}
	DBPolicyDefaultsAllMap = dbPolicyDefaultMap
	logger.Infof("Loaded %d dbpolicydefaults", len(dbPolicyDefaults))
	return nil
}

func loadDBGroupListPoliciesAll(repo repository.DBGroupListPoliciesRepository) error {
	policies, err := repo.GetAll(nil)
	if err != nil {
		logger.Errorf("Failed to load all dbgroup_listpolicies: %v", err)
		return fmt.Errorf("failed to load all dbgroup_listpolicies: %v", err)
	}
	policiesMap := make(map[uint]models.DBGroupListPolicies)
	for _, policy := range policies {
		policiesMap[policy.ID] = policy
	}
	DBGroupListPoliciesAllMap = policiesMap
	logger.Infof("Loaded %d dbgroup_listpolicies", len(policies))
	return nil
}
