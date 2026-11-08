package services

import "github.com/pixelcop/clientshare/internal/access"

// The resolver lives in a dependency-light package so dispatchers can use the
// same policy without cycling through the service/email composition layer.
type AccessService = access.AccessService
type AccessOperation = access.AccessOperation
type AccessActor = access.AccessActor
type AccessRequest = access.AccessRequest
type AccessDecision = access.AccessDecision
type AccessSource = access.AccessSource

const (
	AccessContext      = access.AccessContext
	AccessRead         = access.AccessRead
	AccessList         = access.AccessList
	AccessDownload     = access.AccessDownload
	AccessZIP          = access.AccessZIP
	AccessUpload       = access.AccessUpload
	AccessDelete       = access.AccessDelete
	AccessShare        = access.AccessShare
	AccessInviteClient = access.AccessInviteClient
	AccessManageClient = access.AccessManageClient
	AccessManageQueue  = access.AccessManageQueue
	maxAccessItems     = access.MaxItems
	maxAccessDepth     = access.MaxDepth
)
