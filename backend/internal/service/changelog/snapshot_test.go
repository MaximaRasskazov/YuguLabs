package changelog_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
)

func TestDiff_OnlyChangedFields(t *testing.T) {
	before := changelog.Fields{"a": "1", "b": "2", "c": "3"}
	after := changelog.Fields{"a": "1", "b": "20", "d": "4"}

	diff := changelog.Diff(before, after)

	// a не менялось → его нет в диффе.
	_, hasA := diff["a"]
	require.False(t, hasA, "неизменившееся поле не должно попадать в changed_fields")

	require.Equal(t, changelog.FieldChange{Old: "2", New: "20"}, diff["b"], "изменённое поле")
	require.Equal(t, changelog.FieldChange{Old: "3", New: nil}, diff["c"], "удалённое поле: new=nil")
	require.Equal(t, changelog.FieldChange{Old: nil, New: "4"}, diff["d"], "добавленное поле: old=nil")
	require.Len(t, diff, 3)
}

func TestUserSnapshot_NoSecrets(t *testing.T) {
	mid := "Сергеевич"
	grp := "РТ-224"
	u := queries.User{
		Email:        "a@test.local",
		PasswordHash: "$2a$10$secret-hash-must-not-leak",
		FirstName:    "Анна",
		LastName:     "Тарасова",
		MiddleName:   &mid,
		GroupName:    &grp,
		Birthday:     pgtype.Date{Time: time.Date(2003, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}

	snap := changelog.UserSnapshot(u)

	require.Equal(t, "a@test.local", snap["email"])
	require.Equal(t, "Анна", snap["first_name"])
	require.Equal(t, "Тарасова", snap["last_name"])
	require.Equal(t, "Сергеевич", snap["middle_name"])
	require.Equal(t, "РТ-224", snap["group_name"])
	require.Equal(t, "2003-05-01", snap["birthday"])

	// Главное: ни хеша, ни пароля в срезе быть не должно.
	_, hasHash := snap["password_hash"]
	_, hasPass := snap["password"]
	require.False(t, hasHash, "password_hash не должен попадать в change_logs")
	require.False(t, hasPass, "password не должен попадать в change_logs")
}
