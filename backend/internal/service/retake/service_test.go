package retake_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/audit"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/debt"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/discipline"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/retake"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

type fixture struct {
	store *repo.Store
	disc  *discipline.Service
	debt  *debt.Service
	svc   *retake.Service
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(pool.Close)

	store := repo.NewStore(pool)
	auditSvc := audit.New(store)
	changelogSvc := changelog.New(store)
	disc := discipline.New(store, auditSvc, changelogSvc)
	debtSvc := debt.New(store, auditSvc, changelogSvc, disc, nil)
	svc := retake.New(store, auditSvc, changelogSvc, nil)
	return &fixture{store: store, disc: disc, debt: debtSvc, svc: svc}
}

// seedUser создаёт пользователя и возвращает его id.
func seedUser(t *testing.T, store *repo.Store, prefix string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	email := strings.ToLower(prefix) + "+" + time.Now().Format("150405.000000000") + "@test.local"
	u, err := store.CreateUser(ctx, queries.CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$10$placeholder",
		FirstName:    "T",
		LastName:     "U",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo.CleanupUser(ctx, store.Pool(), u.ID)
	})
	return pgutil.UUID(u.ID)
}

// seedDiscipline создаёт дисциплину и привязывает teacher+student.
func seedDiscipline(t *testing.T, f *fixture, prefix string, teacher, student uuid.UUID) uuid.UUID {
	t.Helper()
	suf := time.Now().Format("150405.000000000")
	d, err := f.disc.Create(context.Background(), discipline.CreateInput{
		Name: prefix + "-" + suf, Code: prefix + "-" + suf,
	}, teacher)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = f.store.Pool().Exec(context.Background(), "DELETE FROM disciplines WHERE id = $1", d.ID)
	})
	id := pgutil.UUID(d.ID)
	require.NoError(t, f.disc.AttachTeacher(context.Background(), id, teacher, teacher))
	require.NoError(t, f.disc.AttachStudent(context.Background(), id, student, teacher, discipline.AttachStudentInput{}))
	return id
}

// seedDebt ставит долг студенту от преподавателя.
func seedDebt(t *testing.T, f *fixture, student, teacher, disciplineID uuid.UUID) uuid.UUID {
	t.Helper()
	d, err := f.debt.Create(context.Background(), debt.CreateInput{
		StudentID: student, DisciplineID: disciplineID,
	}, teacher)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = f.store.Pool().Exec(context.Background(), "DELETE FROM debts WHERE id = $1", d.ID)
	})
	return pgutil.UUID(d.ID)
}

// createScheduledRetake — helper: создаёт regular-пересдачу на завтра.
func createScheduledRetake(t *testing.T, f *fixture, disciplineID, creator uuid.UUID, kind string) queries.Retake {
	t.Helper()
	r, err := f.svc.Create(context.Background(), retake.CreateInput{
		DisciplineID:    disciplineID,
		Kind:            kind,
		Building:        "А",
		Room:            "101",
		ScheduledAt:     retake.NewTime(time.Now().Add(24 * time.Hour)),
		DurationMinutes: 90,
	}, creator)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = f.store.Pool().Exec(context.Background(), "DELETE FROM retakes WHERE id = $1", r.ID)
	})
	return r
}

func TestRetake_Create_Regular(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-create")
	student := seedUser(t, f.store, "st-create")
	discID := seedDiscipline(t, f, "RC", teacher, student)
	dean := seedUser(t, f.store, "dean-create")

	r := createScheduledRetake(t, f, discID, dean, retake.KindRegular)
	require.Equal(t, "regular", r.Kind)
	require.Equal(t, "scheduled", r.Status)
	require.Equal(t, int32(1), r.MinTeachers, "у regular min_teachers=1")
}

func TestRetake_Create_Commission_RequiresMin3(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-com")
	student := seedUser(t, f.store, "st-com")
	discID := seedDiscipline(t, f, "CM", teacher, student)
	dean := seedUser(t, f.store, "dean-com")

	r := createScheduledRetake(t, f, discID, dean, retake.KindCommission)
	require.Equal(t, "commission", r.Kind)
	require.Equal(t, int32(retake.MinCommissionTeachers), r.MinTeachers,
		"у commission min_teachers=3 (ТЗ)")
}

func TestRetake_Create_RejectsInvalidInputs(t *testing.T) {
	f := setup(t)
	dean := seedUser(t, f.store, "dean-bad")

	_, err := f.svc.Create(context.Background(), retake.CreateInput{
		DisciplineID: uuid.Nil,
		Building:     "A", Room: "1",
		ScheduledAt:     retake.NewTime(time.Now().Add(time.Hour)),
		DurationMinutes: 60,
	}, dean)
	require.ErrorIs(t, err, retake.ErrInvalidInput)

	teacher := seedUser(t, f.store, "tr-bad")
	student := seedUser(t, f.store, "st-bad")
	discID := seedDiscipline(t, f, "BAD", teacher, student)

	// Невалидный kind
	_, err = f.svc.Create(context.Background(), retake.CreateInput{
		DisciplineID: discID,
		Kind:         "unknown",
		Building:     "A", Room: "1",
		ScheduledAt:     retake.NewTime(time.Now().Add(time.Hour)),
		DurationMinutes: 60,
	}, dean)
	require.ErrorIs(t, err, retake.ErrInvalidKind)

	// Отрицательная длительность
	_, err = f.svc.Create(context.Background(), retake.CreateInput{
		DisciplineID: discID,
		Building:     "A", Room: "1",
		ScheduledAt:     retake.NewTime(time.Now().Add(time.Hour)),
		DurationMinutes: -5,
	}, dean)
	require.ErrorIs(t, err, retake.ErrInvalidInput)
}

func TestRetake_AddStudent_LinksDebt(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-as")
	student := seedUser(t, f.store, "st-as")
	dean := seedUser(t, f.store, "dean-as")
	discID := seedDiscipline(t, f, "AS", teacher, student)
	debtID := seedDebt(t, f, student, teacher, discID)
	r := createScheduledRetake(t, f, discID, dean, retake.KindRegular)

	require.NoError(t, f.svc.AddStudent(context.Background(), pgutil.UUID(r.ID), student, debtID, dean))

	parts, err := f.svc.ListParticipants(context.Background(), pgutil.UUID(r.ID))
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, "student", parts[0].Kind)
	require.True(t, parts[0].DebtID.Valid)
	require.Equal(t, debtID, pgutil.UUID(parts[0].DebtID))
}

func TestRetake_AddStudent_RejectsForeignDebt(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-fd")
	studentA := seedUser(t, f.store, "stA-fd")
	studentB := seedUser(t, f.store, "stB-fd")
	dean := seedUser(t, f.store, "dean-fd")
	discID := seedDiscipline(t, f, "FD", teacher, studentA)
	// Долг studentA — пытаемся подсунуть его в участие studentB
	debtIDOfA := seedDebt(t, f, studentA, teacher, discID)
	r := createScheduledRetake(t, f, discID, dean, retake.KindRegular)

	err := f.svc.AddStudent(context.Background(), pgutil.UUID(r.ID), studentB, debtIDOfA, dean)
	require.ErrorIs(t, err, retake.ErrInvalidParticipant)
}

func TestRetake_AddTeacher_UsesCorrectKind(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-at")
	student := seedUser(t, f.store, "st-at")
	dean := seedUser(t, f.store, "dean-at")
	discID := seedDiscipline(t, f, "AT", teacher, student)

	// Regular: teacher → kind=teacher
	r1 := createScheduledRetake(t, f, discID, dean, retake.KindRegular)
	require.NoError(t, f.svc.AddTeacher(context.Background(), pgutil.UUID(r1.ID), teacher, dean))
	parts, _ := f.svc.ListParticipants(context.Background(), pgutil.UUID(r1.ID))
	require.Len(t, parts, 1)
	require.Equal(t, "teacher", parts[0].Kind)

	// Commission: teacher → kind=commission_member
	r2 := createScheduledRetake(t, f, discID, dean, retake.KindCommission)
	require.NoError(t, f.svc.AddTeacher(context.Background(), pgutil.UUID(r2.ID), teacher, dean))
	parts2, _ := f.svc.ListParticipants(context.Background(), pgutil.UUID(r2.ID))
	require.Len(t, parts2, 1)
	require.Equal(t, "commission_member", parts2[0].Kind)
}

func TestRetake_Start_RequiresEnoughTeachers(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-s")
	student := seedUser(t, f.store, "st-s")
	dean := seedUser(t, f.store, "dean-s")
	discID := seedDiscipline(t, f, "ST", teacher, student)

	// Commission — нужно 3 преподавателя
	r := createScheduledRetake(t, f, discID, dean, retake.KindCommission)
	// Без преподавателей — нельзя стартовать
	err := f.svc.Start(context.Background(), pgutil.UUID(r.ID), dean)
	require.ErrorIs(t, err, retake.ErrNotEnoughTeachers)

	// Добавляем одного — всё ещё нельзя
	require.NoError(t, f.svc.AddTeacher(context.Background(), pgutil.UUID(r.ID), teacher, dean))
	err = f.svc.Start(context.Background(), pgutil.UUID(r.ID), dean)
	require.ErrorIs(t, err, retake.ErrNotEnoughTeachers)

	// Добавляем ещё двух
	t2 := seedUser(t, f.store, "tr2-s")
	t3 := seedUser(t, f.store, "tr3-s")
	require.NoError(t, f.svc.AddTeacher(context.Background(), pgutil.UUID(r.ID), t2, dean))
	require.NoError(t, f.svc.AddTeacher(context.Background(), pgutil.UUID(r.ID), t3, dean))

	// Теперь можно стартовать
	require.NoError(t, f.svc.Start(context.Background(), pgutil.UUID(r.ID), dean))

	got, _ := f.svc.Get(context.Background(), pgutil.UUID(r.ID))
	require.Equal(t, "in_progress", got.Status)
}

// TestRetake_UpdateSchedule_OnlyScheduled проверяет, что расписание можно
// править только до начала пересдачи. in_progress / completed / cancelled
// редактировать нельзя (баг: декан переносил уже идущую пересдачу).
func TestRetake_UpdateSchedule_OnlyScheduled(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-upd")
	student := seedUser(t, f.store, "st-upd")
	dean := seedUser(t, f.store, "dean-upd")
	discID := seedDiscipline(t, f, "UPD", teacher, student)

	newRoom := "303"
	in := retake.UpdateScheduleInput{Room: &newRoom}

	// scheduled — правка проходит.
	r := createScheduledRetake(t, f, discID, dean, retake.KindRegular)
	_, err := f.svc.UpdateSchedule(context.Background(), pgutil.UUID(r.ID), in, dean)
	require.NoError(t, err)

	// in_progress — правка запрещена.
	require.NoError(t, f.svc.AddTeacher(context.Background(), pgutil.UUID(r.ID), teacher, dean))
	require.NoError(t, f.svc.Start(context.Background(), pgutil.UUID(r.ID), dean))
	_, err = f.svc.UpdateSchedule(context.Background(), pgutil.UUID(r.ID), in, dean)
	require.ErrorIs(t, err, retake.ErrInvalidStatus)

	// completed — правка запрещена.
	require.NoError(t, f.svc.Complete(context.Background(), pgutil.UUID(r.ID), dean))
	_, err = f.svc.UpdateSchedule(context.Background(), pgutil.UUID(r.ID), in, dean)
	require.ErrorIs(t, err, retake.ErrInvalidStatus)

	// cancelled — правка запрещена.
	r2 := createScheduledRetake(t, f, discID, dean, retake.KindRegular)
	require.NoError(t, f.svc.Cancel(context.Background(), pgutil.UUID(r2.ID), dean))
	_, err = f.svc.UpdateSchedule(context.Background(), pgutil.UUID(r2.ID), in, dean)
	require.ErrorIs(t, err, retake.ErrInvalidStatus)
}

// Выставление оценок переехало в пакет statement (двухэтапная
// ведомость). Тесты grade-flow теперь в internal/service/statement.

func TestRetake_Cancel_ReleasesRetake(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-c")
	student := seedUser(t, f.store, "st-c")
	dean := seedUser(t, f.store, "dean-c")
	discID := seedDiscipline(t, f, "CN", teacher, student)
	r := createScheduledRetake(t, f, discID, dean, retake.KindRegular)

	require.NoError(t, f.svc.Cancel(context.Background(), pgutil.UUID(r.ID), dean))

	got, _ := f.svc.Get(context.Background(), pgutil.UUID(r.ID))
	require.Equal(t, "cancelled", got.Status)

	// Повторная отмена — ErrInvalidStatus
	err := f.svc.Cancel(context.Background(), pgutil.UUID(r.ID), dean)
	require.ErrorIs(t, err, retake.ErrInvalidStatus)
}

func TestRetake_ListForUser_OnlyOwn(t *testing.T) {
	f := setup(t)
	teacher := seedUser(t, f.store, "tr-l")
	studentA := seedUser(t, f.store, "stA-l")
	studentB := seedUser(t, f.store, "stB-l")
	dean := seedUser(t, f.store, "dean-l")
	discID := seedDiscipline(t, f, "LU", teacher, studentA)
	debtA := seedDebt(t, f, studentA, teacher, discID)

	r := createScheduledRetake(t, f, discID, dean, retake.KindRegular)
	require.NoError(t, f.svc.AddStudent(context.Background(), pgutil.UUID(r.ID), studentA, debtA, dean))

	rowsA, err := f.svc.ListForUser(context.Background(), studentA)
	require.NoError(t, err)
	require.Len(t, rowsA, 1)

	rowsB, err := f.svc.ListForUser(context.Background(), studentB)
	require.NoError(t, err)
	require.Empty(t, rowsB)
}
