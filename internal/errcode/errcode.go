package errcode

// Code 是面向客户端的稳定错误码。
type Code string

const (
	CodeNameConflict            Code = "NAME_CONFLICT"
	CodeVersionExists           Code = "VERSION_EXISTS"
	CodeInvalidVersion          Code = "INVALID_VERSION"
	CodeInvalidConstraint       Code = "INVALID_CONSTRAINT"
	CodeInvalidManifest         Code = "INVALID_MANIFEST"
	CodeNotFound                Code = "NOT_FOUND"
	CodeCannotDeletePublished   Code = "CANNOT_DELETE_PUBLISHED"
	CodeDependencyTargetMissing Code = "DEPENDENCY_TARGET_MISSING"
	CodeDraftNotPublishable     Code = "DRAFT_NOT_PUBLISHABLE"
	CodeAlreadyPublished        Code = "ALREADY_PUBLISHED"
	CodeAlreadyDeprecated       Code = "ALREADY_DEPRECATED"
	CodeCannotDeprecateDraft    Code = "CANNOT_DEPRECATE_DRAFT"
	CodeNotReady                Code = "NOT_READY"
	CodeInternal                Code = "INTERNAL"
	CodeInvalidArgument         Code = "INVALID_ARGUMENT"
)

// HTTPStatus 返回该错误码对应的 HTTP 状态码。
func (c Code) HTTPStatus() int {
	switch c {
	case CodeNameConflict, CodeVersionExists, CodeCannotDeletePublished,
		CodeAlreadyPublished, CodeAlreadyDeprecated:
		return 409
	case CodeInvalidVersion, CodeInvalidConstraint, CodeInvalidManifest,
		CodeInvalidArgument:
		return 400
	case CodeNotFound:
		return 404
	case CodeDependencyTargetMissing, CodeDraftNotPublishable, CodeCannotDeprecateDraft:
		return 422
	case CodeNotReady:
		return 503
	case CodeInternal:
		return 500
	default:
		return 500
	}
}
