package service

import (
	"context"
	"strconv"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/model"
)

// ListChanges 分页列出变更记录，可按实体类型与 ID 过滤，供审计回溯。
func (s *Service) ListChanges(ctx context.Context, limit, offset int, entityType, entityIDStr string) ([]model.ChangeRecord, error) {
	var entityID int64 = -1
	if entityIDStr != "" {
		id, err := strconv.ParseInt(entityIDStr, 10, 64)
		if err != nil || id < 0 {
			return nil, newAPIError(errcode.CodeInvalidArgument, "invalid entity_id")
		}
		entityID = id
	}
	records, err := s.st.ListChangesPage(limit, offset, entityType, entityID)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return records, nil
}
