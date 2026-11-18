package api

// Error codes the backend puts in the error_code field of its error bodies, by name. The numbers
// are the ones in the backend's util/error_codes.py.
const (
	CodeInvalidPlatform         = 1019
	CodeMalformedChatID         = 1030
	CodeInsufficientCredits     = 1034
	CodeInvalidTTL              = 1043
	CodeTTLTooLong              = 1044
	CodeInvalidDevice           = 1045
	CodeInvalidNotification     = 1046
	CodeUserNotFound            = 2001
	CodeChatNotFound            = 2002
	CodeConnectKeyNotFound      = 2015
	CodeNotChatAdmin            = 3001
	CodeNotTargetUser           = 3002
	CodeNoPrivateChat           = 3004
	CodeWaitlistNotActive       = 3007
	CodePoliciesRequired        = 3008
	CodeNotChatMember           = 3009
	CodeDeliveryRefused         = 3010
	CodeInvalidToken            = 4004
	CodeAccessRevoked           = 4005
	CodeNotificationUndelivered = 5018
	CodeProviderRateLimited     = 6004
)
