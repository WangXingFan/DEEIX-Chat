package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"time"

	auditapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/audit"
	userapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/user"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	rosterImportSource       = "roster_csv"
	rosterPasswordHashCost   = 12
	rosterDefaultTimezone    = "Asia/Shanghai"
	rosterDefaultLocale      = "zh-CN"
	rosterAppearanceDefaults = `{"chatFont":"default","chatFontWeight":"regular","fontSize":"standard","preset":"default","theme":"system"}`
	// RosterMaxRows 是单次导入接受的最大数据行数（不含表头）。
	RosterMaxRows = 5000
	// RosterMaxBytes 是单个名单文件的大小上限。
	RosterMaxBytes = 4 << 20
	// RosterMaxIssues 是结果中逐行明细的最大条数，超出只保留计数。
	RosterMaxIssues = 500
)

// RosterRowStatus 是名单中单行的处理结论。
type RosterRowStatus string

const (
	// RosterRowReady 表示该行可以创建账号。
	RosterRowReady RosterRowStatus = "ready"
	// RosterRowImported 表示该行已成功创建账号。
	RosterRowImported RosterRowStatus = "imported"
	// RosterRowSkippedExisting 表示用户名或邮箱已存在，按要求跳过且不覆盖。
	RosterRowSkippedExisting RosterRowStatus = "skipped_existing"
	// RosterRowDuplicate 表示同一文件内用户名或邮箱重复。
	RosterRowDuplicate RosterRowStatus = "duplicate_in_file"
	// RosterRowInvalid 表示该行未通过字段校验。
	RosterRowInvalid RosterRowStatus = "invalid"
	// RosterRowConflict 表示创建时与并发写入冲突，账号未被创建。
	RosterRowConflict RosterRowStatus = "conflict"
	// RosterRowFailed 表示创建时发生未分类错误。
	RosterRowFailed RosterRowStatus = "failed"
)

// RosterImportInput 描述一次学生名单导入请求。
type RosterImportInput struct {
	// CSV 是客户端上传的名单内容，列顺序固定为 username,displayName,email。
	CSV []byte
	// DryRun 为 true 时只做校验预览，不写入任何账号。
	DryRun bool
}

// RosterImportRow 是名单中一行的处理结果。
type RosterImportRow struct {
	Line        int
	Username    string
	DisplayName string
	Email       string
	Status      RosterRowStatus
	Message     string
}

// RosterCredential 是一次性交付的初始凭据；仅在正式导入成功时返回。
type RosterCredential struct {
	Username        string
	DisplayName     string
	InitialPassword string
}

// RosterImportResult 汇总名单导入结果。
type RosterImportResult struct {
	Total       int
	Ready       int
	Imported    int
	Skipped     int
	Invalid     int
	Failed      int
	Truncated   bool
	Rows        []RosterImportRow
	Credentials []RosterCredential
}

type rosterCandidate struct {
	line        int
	username    string
	displayName string
	email       string
}

type rosterImportUserService interface {
	ListUsersByLowerEmails(ctx context.Context, emails []string) (map[string]domainuser.User, error)
	ListAllUsernames(ctx context.Context) ([]string, error)
	ImportUsersWithCredentialsAndBalances(ctx context.Context, records []repository.UserImportRecord) ([]domainuser.User, error)
}

// ImportRoster 导入管理员提供的学生名单 CSV。
// 预览与正式导入走同一套校验；正式导入逐行创建，单行冲突不影响其余有效行。
func (s *Service) ImportRoster(
	ctx context.Context,
	requestID string,
	actorUserID uint,
	input RosterImportInput,
	ip string,
	userAgent string,
) (*RosterImportResult, error) {
	if _, err := s.getActorUser(ctx, actorUserID); err != nil {
		return nil, err
	}
	importUserService, ok := s.userService.(rosterImportUserService)
	if !ok {
		return nil, ErrRosterImportFailed
	}

	result := &RosterImportResult{Rows: make([]RosterImportRow, 0, 16)}
	candidates, err := parseRosterCSV(input.CSV, result)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		if !input.DryRun {
			s.writeRosterImportAudit(ctx, requestID, actorUserID, ip, userAgent, result)
		}
		return result, nil
	}

	existingUsernames, err := importUserService.ListAllUsernames(ctx)
	if err != nil {
		return nil, err
	}
	takenUsernames := make(map[string]struct{}, len(existingUsernames))
	for _, username := range existingUsernames {
		if value := strings.ToLower(strings.TrimSpace(username)); value != "" {
			takenUsernames[value] = struct{}{}
		}
	}

	lowerEmails := rosterUniqueEmails(candidates)
	existingByEmail, err := importUserService.ListUsersByLowerEmails(ctx, lowerEmails)
	if err != nil {
		return nil, err
	}

	for _, candidate := range candidates {
		row := RosterImportRow{
			Line:        candidate.line,
			Username:    candidate.username,
			DisplayName: candidate.displayName,
			Email:       candidate.email,
		}
		if _, exists := takenUsernames[candidate.username]; exists {
			row.Status = RosterRowSkippedExisting
			row.Message = "username already exists"
			result.Skipped++
			result.appendRow(row)
			continue
		}
		if lower := strings.ToLower(candidate.email); lower != "" {
			if _, exists := existingByEmail[lower]; exists {
				row.Status = RosterRowSkippedExisting
				row.Message = "email already exists"
				result.Skipped++
				result.appendRow(row)
				continue
			}
		}
		if input.DryRun {
			row.Status = RosterRowReady
			result.Ready++
			result.appendRow(row)
			continue
		}

		credential, createErr := s.createRosterAccount(ctx, importUserService, candidate)
		switch {
		case createErr == nil:
			row.Status = RosterRowImported
			result.Imported++
			result.Credentials = append(result.Credentials, credential)
			takenUsernames[candidate.username] = struct{}{}
			if lower := strings.ToLower(candidate.email); lower != "" {
				existingByEmail[lower] = domainuser.User{}
			}
		case errors.Is(createErr, repository.ErrDuplicate), errors.Is(createErr, repository.ErrDuplicateUsername):
			row.Status = RosterRowConflict
			row.Message = "username or email already exists"
			result.Skipped++
			takenUsernames[candidate.username] = struct{}{}
		default:
			row.Status = RosterRowFailed
			row.Message = "account creation failed"
			result.Failed++
		}
		result.appendRow(row)
	}

	if !input.DryRun {
		s.writeRosterImportAudit(ctx, requestID, actorUserID, ip, userAgent, result)
	}
	return result, nil
}

// createRosterAccount 为单行名单创建账号，使用独立事务，避免一行冲突回滚整批。
func (s *Service) createRosterAccount(
	ctx context.Context,
	importUserService rosterImportUserService,
	candidate rosterCandidate,
) (RosterCredential, error) {
	password, err := randomRosterPassword()
	if err != nil {
		return RosterCredential{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), rosterPasswordHashCost)
	if err != nil {
		return RosterCredential{}, err
	}
	now := time.Now()
	record := repository.UserImportRecord{
		User: domainuser.User{
			PublicID:              rosterPublicID(),
			Username:              candidate.username,
			DisplayName:           candidate.displayName,
			Email:                 candidate.email,
			EmailSource:           domainuser.EmailSourceAdminSet,
			Role:                  domainuser.RoleUser,
			Status:                domainuser.StatusActive,
			Timezone:              rosterDefaultTimezone,
			Locale:                rosterDefaultLocale,
			AppearancePreferences: rosterAppearanceDefaults,
		},
		Credential: domainuser.Credential{
			PasswordHash:      string(passwordHash),
			PasswordAlgo:      "bcrypt",
			PasswordEnabled:   true,
			PasswordUpdatedAt: &now,
			PasswordSetAt:     &now,
			PasswordOrigin:    domainuser.PasswordOriginAdminCreated,
			// 名单导入的初始密码是一次性凭据：首次登录必须改密。
			MustResetPassword: true,
		},
	}
	if _, err := importUserService.ImportUsersWithCredentialsAndBalances(ctx, []repository.UserImportRecord{record}); err != nil {
		return RosterCredential{}, err
	}
	return RosterCredential{
		Username:        candidate.username,
		DisplayName:     candidate.displayName,
		InitialPassword: password,
	}, nil
}

// parseRosterCSV 解析并逐行校验名单；返回可创建账号的候选，并把结论写入结果。
func parseRosterCSV(raw []byte, result *RosterImportResult) ([]rosterCandidate, error) {
	data := bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > RosterMaxBytes {
		return nil, ErrRosterFileTooLarge
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	reader.ReuseRecord = false

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, ErrInvalidRosterCSV
	}
	if !isRosterHeader(header) {
		return nil, ErrInvalidRosterHeader
	}

	candidates := make([]rosterCandidate, 0, 64)
	seenUsernames := make(map[string]struct{})
	seenEmails := make(map[string]struct{})
	line := 1
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		line++
		if result.Total >= RosterMaxRows {
			return nil, ErrRosterTooManyRows
		}
		if readErr != nil {
			result.Total++
			result.Invalid++
			result.appendRow(RosterImportRow{Line: line, Status: RosterRowInvalid, Message: "malformed csv row"})
			continue
		}
		row, candidate, ok := validateRosterRecord(record, line, seenUsernames, seenEmails)
		result.Total++
		if !ok {
			result.Invalid++
			result.appendRow(row)
			continue
		}
		result.appendRow(row)
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (r *RosterImportResult) appendRow(row RosterImportRow) {
	if len(r.Rows) >= RosterMaxIssues {
		r.Truncated = true
		return
	}
	r.Rows = append(r.Rows, row)
}

func isRosterHeader(header []string) bool {
	if len(header) < 1 {
		return false
	}
	if normalizeRosterHeader(header[0]) != "username" {
		return false
	}
	if len(header) > 1 && normalizeRosterHeader(header[1]) != "displayname" && strings.TrimSpace(header[1]) != "" {
		return false
	}
	if len(header) > 2 && normalizeRosterHeader(header[2]) != "email" && strings.TrimSpace(header[2]) != "" {
		return false
	}
	return true
}

func normalizeRosterHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

// validateRosterRecord 校验单行；status 为 ready 时返回候选。
func validateRosterRecord(
	record []string,
	line int,
	seenUsernames map[string]struct{},
	seenEmails map[string]struct{},
) (RosterImportRow, rosterCandidate, bool) {
	row := RosterImportRow{Line: line}
	rawUsername := ""
	if len(record) > 0 {
		rawUsername = record[0]
	}
	row.Username = strings.TrimSpace(rawUsername)
	if len(record) > 1 {
		row.DisplayName = strings.TrimSpace(record[1])
	}
	if len(record) > 2 {
		row.Email = strings.TrimSpace(record[2])
	}

	if row.Username == "" {
		row.Status = RosterRowInvalid
		row.Message = "username is required"
		return row, rosterCandidate{}, false
	}
	username, err := userapp.NormalizeUsername(row.Username)
	if err != nil {
		row.Status = RosterRowInvalid
		row.Message = "username does not satisfy the platform rules"
		return row, rosterCandidate{}, false
	}
	row.Username = username
	if _, dup := seenUsernames[username]; dup {
		row.Status = RosterRowDuplicate
		row.Message = "username repeats inside this file"
		return row, rosterCandidate{}, false
	}

	displayName := row.DisplayName
	if displayName == "" {
		displayName = username
	}
	normalizedDisplayName, err := userapp.NormalizeDisplayName(displayName)
	if err != nil {
		row.Status = RosterRowInvalid
		row.Message = "displayName does not satisfy the platform rules"
		return row, rosterCandidate{}, false
	}
	row.DisplayName = normalizedDisplayName

	email, err := userapp.NormalizeEmail(row.Email)
	if err != nil {
		row.Status = RosterRowInvalid
		row.Message = "email is not a valid address"
		return row, rosterCandidate{}, false
	}
	row.Email = email
	if lower := strings.ToLower(email); lower != "" {
		if _, dup := seenEmails[lower]; dup {
			row.Status = RosterRowDuplicate
			row.Message = "email repeats inside this file"
			return row, rosterCandidate{}, false
		}
		seenEmails[lower] = struct{}{}
	}
	seenUsernames[username] = struct{}{}

	row.Status = RosterRowReady
	return row, rosterCandidate{
		line:        line,
		username:    username,
		displayName: normalizedDisplayName,
		email:       email,
	}, true
}

// rosterUniqueEmails 返回候选中出现过的唯一小写邮箱，用于一次查询既有账号。
func rosterUniqueEmails(candidates []rosterCandidate) []string {
	lower := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		value := strings.ToLower(strings.TrimSpace(candidate.email))
		if value == "" {
			continue
		}
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		lower = append(lower, value)
	}
	return lower
}

func rosterPublicID() string {
	return strings.ReplaceAll(newRosterUUID(), "-", "")
}

func newRosterUUID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 36)
	pos := 0
	for i, b := range buf {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[pos] = '-'
			pos++
		}
		out[pos] = hexdigits[b>>4]
		out[pos+1] = hexdigits[b&0x0f]
		pos += 2
	}
	return string(out)
}

// randomRosterPassword 生成每行独立的初始密码。
func randomRosterPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Service) writeRosterImportAudit(
	ctx context.Context,
	requestID string,
	actorUserID uint,
	ip string,
	userAgent string,
	result *RosterImportResult,
) {
	if result == nil {
		return
	}
	// 只记录计数，初始密码绝不进入审计详情。
	s.auditService.Write(ctx, auditapp.WriteInput{
		RequestID:   requestID,
		ActorUserID: actorUserID,
		Action:      "admin_import_roster_users",
		Resource:    "user",
		ResourceID:  rosterImportSource,
		IP:          ip,
		UserAgent:   userAgent,
		Detail: map[string]any{
			"source":    rosterImportSource,
			"total":     result.Total,
			"ready":     result.Ready,
			"imported":  result.Imported,
			"skipped":   result.Skipped,
			"invalid":   result.Invalid,
			"failed":    result.Failed,
			"truncated": result.Truncated,
		},
	})
}
