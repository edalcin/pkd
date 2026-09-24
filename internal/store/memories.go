package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/edalcin/pkd/internal/model"
)

// ErrMemoryHierarchy is returned when an operation would give a Memória a
// parent or children. Memórias live only in the MC tree, placed by date.
var ErrMemoryHierarchy = errors.New("memórias não têm pai nem filhos")

// iconMemory is the default icon of a new Memória (boxicons).
const iconMemory = "bx-calendar-event"

// memoryPeriod is a named part of the day; start/length in hours.
type memoryPeriod struct{ start, length int }

// memoryPeriods are project conventions (docs/memoriaCronologica.md). In the
// ID a Período is written as its start hour (ADR-007).
var memoryPeriods = map[string]memoryPeriod{
	"madrugada": {0, 6},
	"manha":     {6, 6},
	"almoco":    {12, 3},
	"tarde":     {12, 6},
	"lanche":    {16, 2},
	"jantar":    {18, 3},
	"noite":     {18, 6},
}

// MemoryDate is the Data da Memória. Only Year is required; finer parts are
// set only when known. Hour/Minute and Period are mutually exclusive.
type MemoryDate struct {
	Year   int    `json:"year"`
	Month  *int   `json:"month,omitempty"`
	Day    *int   `json:"day,omitempty"`
	Hour   *int   `json:"hour,omitempty"`
	Minute *int   `json:"minute,omitempty"`
	Period string `json:"period,omitempty"`
}

// Validate rejects impossible dates before an ID is emitted (e.g. 31/02).
func (d MemoryDate) Validate() error {
	if d.Year < 1 || d.Year > 9999 {
		return errors.New("year must be between 1 and 9999")
	}
	if d.Month != nil && (*d.Month < 1 || *d.Month > 12) {
		return errors.New("month must be between 1 and 12")
	}
	if d.Day != nil {
		if d.Month == nil {
			return errors.New("day requires month")
		}
		last := time.Date(d.Year, time.Month(*d.Month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if *d.Day < 1 || *d.Day > last {
			return fmt.Errorf("day must be between 1 and %d for this month", last)
		}
	}
	if (d.Hour != nil || d.Period != "") && d.Day == nil {
		return errors.New("hour or period requires day")
	}
	if d.Hour != nil && d.Period != "" {
		return errors.New("hour and period are mutually exclusive")
	}
	if d.Hour != nil && (*d.Hour < 0 || *d.Hour > 23) {
		return errors.New("hour must be between 0 and 23")
	}
	if d.Minute != nil {
		if d.Hour == nil {
			return errors.New("minute requires hour")
		}
		if *d.Minute < 0 || *d.Minute > 59 {
			return errors.New("minute must be between 0 and 59")
		}
	}
	if d.Period != "" {
		if _, ok := memoryPeriods[d.Period]; !ok {
			return fmt.Errorf("unknown period %q", d.Period)
		}
	}
	return nil
}

// idPrefix projects the date into MEM-AAAA[-MM[-DD[T<HH>[MM]]]].
func (d MemoryDate) idPrefix() string {
	s := fmt.Sprintf("MEM-%04d", d.Year)
	if d.Month == nil {
		return s
	}
	s += fmt.Sprintf("-%02d", *d.Month)
	if d.Day == nil {
		return s
	}
	s += fmt.Sprintf("-%02d", *d.Day)
	switch {
	case d.Hour != nil && d.Minute != nil:
		s += fmt.Sprintf("T%02d%02d", *d.Hour, *d.Minute)
	case d.Hour != nil:
		s += fmt.Sprintf("T%02d", *d.Hour)
	case d.Period != "":
		s += fmt.Sprintf("T%02d", memoryPeriods[d.Period].start)
	}
	return s
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newMemoryID emits a fresh ID: date prefix + 6 random Crockford chars.
func newMemoryID(d MemoryDate) (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = crockford[b[i]&31] // 256 % 32 == 0 → uniform
	}
	return d.idPrefix() + "-" + string(b[:]), nil
}

var memoryIDRe = regexp.MustCompile(`^(?:[A-Z][A-Z0-9]{1,3}-)?[0-9]{4}(?:-[0-9]{2}(?:-[0-9]{2}(?:T[0-9]{2}(?:[0-9]{2})?)?)?)?-[0-9A-HJKMNP-TV-Z]{6}$`)

// NormalizeMemoryID returns the canonical (uppercase) form of an ID typed in
// any case; Crockford decoding maps I/L→1 and O→0 in the suffix.
func NormalizeMemoryID(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	i := strings.LastIndexByte(s, '-')
	if i < 0 {
		return "", false
	}
	suffix := strings.NewReplacer("I", "1", "L", "1", "O", "0").Replace(s[i+1:])
	s = s[:i+1] + suffix
	return s, memoryIDRe.MatchString(s)
}

// uniqueTitle returns title, or "title (N)" if an active document holds it.
func uniqueTitle(tx *sql.Tx, title string) string {
	actual := title
	for n := 2; n <= 200; n++ {
		var cnt int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM documents WHERE title = ? COLLATE NOCASE AND trashed_at IS NULL`,
			actual,
		).Scan(&cnt); err != nil || cnt == 0 {
			break
		}
		actual = fmt.Sprintf("%s (%d)", title, n)
	}
	return actual
}

type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func isMemory(q rowQuerier, id int64) bool {
	var n int
	_ = q.QueryRow(`SELECT COUNT(*) FROM documents WHERE id = ? AND memory_id IS NOT NULL`, id).Scan(&n)
	return n > 0
}

func rejectMemoryParent(tx *sql.Tx, parentID *int64) error {
	if parentID != nil && isMemory(tx, *parentID) {
		return ErrMemoryHierarchy
	}
	return nil
}

func rejectMemoryMove(tx *sql.Tx, id int64, newParentID *int64) error {
	if isMemory(tx, id) {
		return ErrMemoryHierarchy
	}
	return rejectMemoryParent(tx, newParentID)
}

func scanMemoryFields(q rowQuerier, id int64, doc *model.Document) error {
	var mid, period sql.NullString
	var hour, minute sql.NullInt64
	if err := q.QueryRow(
		`SELECT memory_id, memory_hour, memory_minute, memory_period FROM documents WHERE id = ?`, id,
	).Scan(&mid, &hour, &minute, &period); err != nil {
		return err
	}
	doc.MemoryID = mid.String
	doc.MemoryPeriod = period.String
	doc.MemoryHour, doc.MemoryMinute = nullIntPtr(hour), nullIntPtr(minute)
	return nil
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

// CreateMemory inserts a Memória. A non-empty key already used returns the
// existing Memória with created=false (client idempotency).
func (s *DocumentStore) CreateMemory(title string, d MemoryDate, key string) (doc *model.Document, created bool, err error) {
	var id int64
	err = WithTx(s.db, func(tx *sql.Tx) error {
		if key != "" {
			err := tx.QueryRow(`SELECT id FROM documents WHERE memory_key = ? AND trashed_at IS NULL`, key).Scan(&id)
			if err == nil {
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var keyArg any
		if key != "" {
			keyArg = key
		}
		title = uniqueTitle(tx, title)
		// Suffix collision: retry with a fresh suffix (spec rule 5).
		for range 5 {
			mid, err := newMemoryID(d)
			if err != nil {
				return err
			}
			res, err := tx.Exec(`
				INSERT INTO documents (parent_id, title, icon, position, created_at, updated_at,
				    assoc_year, assoc_month, assoc_day, memory_id, memory_hour, memory_minute, memory_period, memory_key)
				VALUES (NULL, ?, ?, 0, `+nowISO+`, `+nowISO+`, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
				title, iconMemory, d.Year, d.Month, d.Day, mid, d.Hour, d.Minute, d.Period, keyArg)
			if err != nil && strings.Contains(err.Error(), "documents.memory_id") {
				continue
			}
			if err != nil {
				return fmt.Errorf("insert memory: %w", err)
			}
			id, _ = res.LastInsertId()
			created = true
			return nil
		}
		return errors.New("memory id collision: retries exhausted")
	})
	if err != nil {
		return nil, false, err
	}
	doc, err = s.GetByID(id)
	return doc, created, err
}

// GetByMemoryID returns the active Memória with the given canonical ID.
func (s *DocumentStore) GetByMemoryID(memoryID string) (*model.Document, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM documents WHERE memory_id = ? AND trashed_at IS NULL`, memoryID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

// UpdateMemoryDate replaces the whole Data da Memória. The memory_id is never
// re-emitted (ADR-007). Does not bump version, like UpdateAssocDate.
func (s *DocumentStore) UpdateMemoryDate(id int64, d MemoryDate) (*model.Document, error) {
	res, err := s.db.Exec(`
		UPDATE documents SET assoc_year = ?, assoc_month = ?, assoc_day = ?,
		    memory_hour = ?, memory_minute = ?, memory_period = NULLIF(?, '')
		WHERE id = ? AND memory_id IS NOT NULL AND trashed_at IS NULL`,
		d.Year, d.Month, d.Day, d.Hour, d.Minute, d.Period, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetByID(id)
}

// MemoryListItem is one line of the MC tree (GET /api/memories).
type MemoryListItem struct {
	ID        int64  `json:"id"`
	MemoryID  string `json:"memory_id"`
	Title     string `json:"title"`
	Icon      string `json:"icon"`
	Year      int    `json:"year"`
	Month     *int   `json:"month"`
	Day       *int   `json:"day"`
	Hour      *int   `json:"hour"`
	Minute    *int   `json:"minute"`
	Period    string `json:"period"`
	CreatedAt string `json:"-"`
}

// ListMemories returns active (non-trashed, non-archived) Memórias in MC
// tree order (see memoryLess).
func (s *DocumentStore) ListMemories() ([]MemoryListItem, error) {
	rows, err := s.db.Query(`
		SELECT id, memory_id, title, COALESCE(icon, ''), assoc_year, assoc_month, assoc_day,
		       memory_hour, memory_minute, COALESCE(memory_period, ''), created_at
		FROM documents
		WHERE memory_id IS NOT NULL AND trashed_at IS NULL AND archived_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemoryListItem{}
	for rows.Next() {
		var m MemoryListItem
		var month, day, hour, minute sql.NullInt64
		if err := rows.Scan(&m.ID, &m.MemoryID, &m.Title, &m.Icon, &m.Year, &month, &day,
			&hour, &minute, &m.Period, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Month, m.Day, m.Hour, m.Minute = nullIntPtr(month), nullIntPtr(day), nullIntPtr(hour), nullIntPtr(minute)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return memoryLess(out[i], out[j]) })
	return out, nil
}

// memoryLess orders the MC tree: years, months and days newest first, with
// the coarser Memória (no month / no day) before the finer ones; inside a
// day chronological: no time first, then start, then longer span first
// (tarde before almoço before exact 12:00), then creation.
func memoryLess(a, b MemoryListItem) bool {
	if a.Year != b.Year {
		return a.Year > b.Year
	}
	if c := cmpDesc(a.Month, b.Month); c != 0 {
		return c < 0
	}
	if c := cmpDesc(a.Day, b.Day); c != 0 {
		return c < 0
	}
	as, al, at := daySlot(a)
	bs, bl, bt := daySlot(b)
	if at != bt {
		return !at // no time first
	}
	if as != bs {
		return as < bs
	}
	if al != bl {
		return al > bl
	}
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt < b.CreatedAt
	}
	return a.ID < b.ID
}

// cmpDesc: nil first, then descending. <0 means a before b.
func cmpDesc(a, b *int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return *b - *a
}

// daySlot returns start minute, span in minutes (0 = exact time) and whether
// the Memória has any time at all.
func daySlot(m MemoryListItem) (start, span int, hasTime bool) {
	if p, ok := memoryPeriods[m.Period]; ok {
		return p.start * 60, p.length * 60, true
	}
	if m.Hour != nil {
		start = *m.Hour * 60
		if m.Minute != nil {
			start += *m.Minute
		}
		return start, 0, true
	}
	return 0, 0, false
}

// MemoryDocIDs returns the document IDs of all Memórias, so search results
// (which mix Memórias and Documentos) can mark them.
func (s *DocumentStore) MemoryDocIDs() (map[int64]bool, error) {
	rows, err := s.db.Query(`SELECT id FROM documents WHERE memory_id IS NOT NULL AND trashed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
