package input

// Application holds the inbound ports of the HTTP process.
type Application struct {
	Health       HealthChecker
	Authorizer   Authorizer
	Users        UserService
	Auth         AuthService
	Account      AccountService
	Records      RecordService
	Collections  CollectionService
	Audit        AuditService
	Integrations IntegrationService
	Tools        ToolsService
}
