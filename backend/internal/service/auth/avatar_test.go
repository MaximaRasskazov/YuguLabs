package auth_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
)

// TestAuth_SetAvatar_Validation проверяет валидацию без БД: пустой файл,
// неверный тип и превышение размера отсекаются до обращения к хранилищу,
// поэтому store здесь не нужен (nil).
func TestAuth_SetAvatar_Validation(t *testing.T) {
	svc := auth.New(nil, nil, nil)
	ctx := context.Background()
	id := uuid.New()

	require.ErrorIs(t, svc.SetAvatar(ctx, id, nil), auth.ErrAvatarEmpty)

	require.ErrorIs(t, svc.SetAvatar(ctx, id, []byte("это просто текст, не картинка")), auth.ErrAvatarType)

	big := make([]byte, auth.MaxAvatarBytes+1)
	require.ErrorIs(t, svc.SetAvatar(ctx, id, big), auth.ErrAvatarTooLarge)
}
