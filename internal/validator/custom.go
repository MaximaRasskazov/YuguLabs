package validator

import (
	"regexp"
	"time"

	"github.com/go-playground/validator/v10"
)

var ValidateUsername validator.Func = func(fl validator.FieldLevel) bool {
	username := fl.Field().String()
	matched, _ := regexp.MatchString(`^[A-Z][a-zA-Z]*$`, username)
	return matched
}

var ValidatePassword validator.Func = func(fl validator.FieldLevel) bool {
	password := fl.Field().String()

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	hasSpecial := regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password)

	return hasUpper && hasLower && hasDigit && hasSpecial
}

var ValidateAge14 validator.Func = func(fl validator.FieldLevel) bool {
	birthdayStr := fl.Field().String()

	birthday, err := time.Parse("2006-01-02", birthdayStr)
	if err != nil {
		return false
	}

	fourteenYearsAgo := time.Now().AddDate(-14, 0, 0)

	return birthday.Before(fourteenYearsAgo) || birthday.Equal(fourteenYearsAgo)
}

// ValidateSlugFormat проверяет, что строка состоит только из a-z, A-Z, 0-9, - и _
var ValidateSlugFormat validator.Func = func(fl validator.FieldLevel) bool {
	slug := fl.Field().String()

	// Регулярное выражение от начала ^ до конца $ строки
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, slug)
	return matched
}
