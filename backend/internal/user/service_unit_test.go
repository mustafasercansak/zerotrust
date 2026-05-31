package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type fakeStore struct {
	createUser             *User
	createErr              error
	createEmail            string
	createPasswordHash     string
	createLocale           string
	createWithRolesUser    *User
	createWithRolesErr     error
	createWithRolesEmail   string
	createWithRolesHash    string
	createWithRolesLocale  string
	createWithRolesRoles   []string
	findByEmailUser        *User
	findByEmailErr         error
	findByEmailInput       string
	findByIDUser           *User
	findByIDErr            error
	findByIDInput          string
	listResult             ListResult
	listErr                error
	setRolesErr            error
	setRolesUserID         string
	setRolesRoles          []string
	setActiveErr           error
	setActiveUserID        string
	setActiveValue         bool
	updateProfileUser      *User
	updateProfileErr       error
	updateProfileUserID    string
	updateProfileFirstName string
	updateProfileLastName  string
	permissions            []string
	permissionsErr         error
	permissionsUserID      string
	updatePasswordErr      error
	updatePasswordUserID   string
	updatePasswordHash     string
	assignRoleErr          error
	assignRoleUserID       string
	assignRoleName         string
	updateAvatarUser       *User
	updateAvatarErr        error
	updateAvatarUserID     string
	updateAvatarKey        string
	updateAvatarSize       int
}

func (f *fakeStore) Create(_ context.Context, email, passwordHash, locale string) (*User, error) {
	f.createEmail = email
	f.createPasswordHash = passwordHash
	f.createLocale = locale
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createUser, nil
}

func (f *fakeStore) CreateWithRoles(_ context.Context, email, passwordHash, locale string, roles []string) (*User, error) {
	f.createWithRolesEmail = email
	f.createWithRolesHash = passwordHash
	f.createWithRolesLocale = locale
	f.createWithRolesRoles = append([]string(nil), roles...)
	if f.createWithRolesErr != nil {
		return nil, f.createWithRolesErr
	}
	return f.createWithRolesUser, nil
}

func (f *fakeStore) FindByEmail(_ context.Context, email string) (*User, error) {
	f.findByEmailInput = email
	if f.findByEmailErr != nil {
		return nil, f.findByEmailErr
	}
	return f.findByEmailUser, nil
}

func (f *fakeStore) FindByID(_ context.Context, id string) (*User, error) {
	f.findByIDInput = id
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}
	return f.findByIDUser, nil
}

func (f *fakeStore) List(_ context.Context, p ListParams) (ListResult, error) {
	if f.listErr != nil {
		return ListResult{}, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeStore) SetRoles(_ context.Context, userID string, roles []string) error {
	f.setRolesUserID = userID
	f.setRolesRoles = append([]string(nil), roles...)
	return f.setRolesErr
}

func (f *fakeStore) SetActive(_ context.Context, userID string, active bool) error {
	f.setActiveUserID = userID
	f.setActiveValue = active
	return f.setActiveErr
}

func (f *fakeStore) UpdateProfile(_ context.Context, userID, firstName, lastName string) (*User, error) {
	f.updateProfileUserID = userID
	f.updateProfileFirstName = firstName
	f.updateProfileLastName = lastName
	if f.updateProfileErr != nil {
		return nil, f.updateProfileErr
	}
	return f.updateProfileUser, nil
}

func (f *fakeStore) GetPermissions(_ context.Context, userID string) ([]string, error) {
	f.permissionsUserID = userID
	if f.permissionsErr != nil {
		return nil, f.permissionsErr
	}
	return f.permissions, nil
}

func (f *fakeStore) UpdatePassword(_ context.Context, userID, passwordHash string) error {
	f.updatePasswordUserID = userID
	f.updatePasswordHash = passwordHash
	return f.updatePasswordErr
}

func (f *fakeStore) AssignRoleByName(_ context.Context, userID, roleName string) error {
	f.assignRoleUserID = userID
	f.assignRoleName = roleName
	return f.assignRoleErr
}

func (f *fakeStore) UpdateAvatar(_ context.Context, userID, key string, size int) (*User, error) {
	f.updateAvatarUserID = userID
	f.updateAvatarKey = key
	f.updateAvatarSize = size
	if f.updateAvatarErr != nil {
		return nil, f.updateAvatarErr
	}
	return f.updateAvatarUser, nil
}

func TestCheckPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Password1!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}

	svc := &Service{}
	if !svc.CheckPassword(string(hash), "Password1!") {
		t.Fatal("expected password verification to succeed")
	}
	if svc.CheckPassword(string(hash), "wrong") {
		t.Fatal("expected password verification to fail for wrong password")
	}
}

func TestUpdateProfileRejectsLongNames(t *testing.T) {
	svc := &Service{}
	long := strings.Repeat("a", 81)

	_, err := svc.UpdateProfile(context.TODO(), "user-id", long, "ok")
	if err != ErrInvalidProfile {
		t.Fatalf("first name error=%v want=%v", err, ErrInvalidProfile)
	}

	_, err = svc.UpdateProfile(context.TODO(), "user-id", "ok", long)
	if err != ErrInvalidProfile {
		t.Fatalf("last name error=%v want=%v", err, ErrInvalidProfile)
	}
}

func TestRegisterNormalizesEmailAndHashesPassword(t *testing.T) {
	store := &fakeStore{createUser: &User{ID: "u1"}}
	svc := NewService(store)

	_, err := svc.Register(context.TODO(), " Test@Example.com ", "Password1!", "en")
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if store.createEmail != "test@example.com" {
		t.Fatalf("email=%q want=test@example.com", store.createEmail)
	}
	if store.createLocale != "en" {
		t.Fatalf("locale=%q want=en", store.createLocale)
	}
	if store.createPasswordHash == "" || store.createPasswordHash == "Password1!" {
		t.Fatal("expected hashed password to be stored")
	}
	if !svc.CheckPassword(store.createPasswordHash, "Password1!") {
		t.Fatal("stored hash does not match original password")
	}
}

func TestRegisterWithRolesNormalizesEmailAndHashesPassword(t *testing.T) {
	store := &fakeStore{createWithRolesUser: &User{ID: "u1"}}
	svc := NewService(store)

	_, err := svc.RegisterWithRoles(context.TODO(), " Admin@Example.com ", "Password1!", "tr", []string{"admin"})
	if err != nil {
		t.Fatalf("RegisterWithRoles error: %v", err)
	}
	if store.createWithRolesEmail != "admin@example.com" {
		t.Fatalf("email=%q want=admin@example.com", store.createWithRolesEmail)
	}
	if len(store.createWithRolesRoles) != 1 || store.createWithRolesRoles[0] != "admin" {
		t.Fatalf("roles=%v", store.createWithRolesRoles)
	}
	if store.createWithRolesHash == "" || store.createWithRolesHash == "Password1!" {
		t.Fatal("expected hashed password to be stored")
	}
}

func TestFindersAndPassthroughs(t *testing.T) {
	store := &fakeStore{
		findByEmailUser:   &User{ID: "u1"},
		findByIDUser:      &User{ID: "u2"},
		listResult:        ListResult{Total: 3},
		permissions:       []string{"users:read"},
		updateAvatarUser:  &User{ID: "u3", HasAvatar: true},
		updateProfileUser: &User{ID: "u4", FirstName: "Jane", LastName: "Doe"},
	}
	svc := NewService(store)

	if _, err := svc.FindByEmail(context.TODO(), " User@Example.com "); err != nil {
		t.Fatalf("FindByEmail error: %v", err)
	}
	if store.findByEmailInput != "user@example.com" {
		t.Fatalf("findByEmailInput=%q want=user@example.com", store.findByEmailInput)
	}

	if _, err := svc.FindByID(context.TODO(), "u2"); err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if store.findByIDInput != "u2" {
		t.Fatalf("findByIDInput=%q want=u2", store.findByIDInput)
	}

	if result, err := svc.List(context.TODO(), ListParams{Limit: 10}); err != nil || result.Total != 3 {
		t.Fatalf("List result=%+v err=%v", result, err)
	}

	if perms, err := svc.GetPermissions(context.TODO(), "u9"); err != nil || len(perms) != 1 {
		t.Fatalf("GetPermissions perms=%v err=%v", perms, err)
	}
	if store.permissionsUserID != "u9" {
		t.Fatalf("permissions user id=%q want=u9", store.permissionsUserID)
	}

	avatarUser, err := svc.UpdateAvatar(context.TODO(), "u3", "avatar-key", 42)
	if err != nil || !avatarUser.HasAvatar {
		t.Fatalf("UpdateAvatar user=%+v err=%v", avatarUser, err)
	}
	if store.updateAvatarUserID != "u3" || store.updateAvatarKey != "avatar-key" || store.updateAvatarSize != 42 {
		t.Fatalf("unexpected update avatar call: user=%q key=%q size=%d", store.updateAvatarUserID, store.updateAvatarKey, store.updateAvatarSize)
	}

	profileUser, err := svc.UpdateProfile(context.TODO(), "u4", " Jane ", " Doe ")
	if err != nil || profileUser.FirstName != "Jane" {
		t.Fatalf("UpdateProfile user=%+v err=%v", profileUser, err)
	}
	if store.updateProfileFirstName != "Jane" || store.updateProfileLastName != "Doe" {
		t.Fatalf("trimmed names not passed: first=%q last=%q", store.updateProfileFirstName, store.updateProfileLastName)
	}
}

func TestSettersPassthrough(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	if err := svc.SetRoles(context.TODO(), "u1", []string{"admin", "viewer"}); err != nil {
		t.Fatalf("SetRoles error: %v", err)
	}
	if store.setRolesUserID != "u1" || len(store.setRolesRoles) != 2 {
		t.Fatalf("unexpected set roles call: user=%q roles=%v", store.setRolesUserID, store.setRolesRoles)
	}

	if err := svc.SetActive(context.TODO(), "u1", false); err != nil {
		t.Fatalf("SetActive error: %v", err)
	}
	if store.setActiveUserID != "u1" || store.setActiveValue != false {
		t.Fatalf("unexpected set active call: user=%q active=%v", store.setActiveUserID, store.setActiveValue)
	}

	if err := svc.UpdatePassword(context.TODO(), "u1", "hash"); err != nil {
		t.Fatalf("UpdatePassword error: %v", err)
	}
	if store.updatePasswordUserID != "u1" || store.updatePasswordHash != "hash" {
		t.Fatalf("unexpected update password call: user=%q hash=%q", store.updatePasswordUserID, store.updatePasswordHash)
	}
}

func TestSeedAdmin(t *testing.T) {
	t.Run("existing user assigns admin role", func(t *testing.T) {
		store := &fakeStore{findByEmailUser: &User{ID: "u1"}}
		svc := NewService(store)

		if err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash"); err != nil {
			t.Fatalf("SeedAdmin error: %v", err)
		}
		if store.assignRoleUserID != "u1" || store.assignRoleName != "admin" {
			t.Fatalf("assign role call mismatch user=%q role=%q", store.assignRoleUserID, store.assignRoleName)
		}
	})

	t.Run("not found creates user then assigns role", func(t *testing.T) {
		store := &fakeStore{
			findByEmailErr: ErrNotFound,
			createUser: &User{
				ID:        "u2",
				Email:     "admin@example.com",
				CreatedAt: time.Now(),
			},
		}
		svc := NewService(store)

		if err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash"); err != nil {
			t.Fatalf("SeedAdmin error: %v", err)
		}
		if store.createEmail != "admin@example.com" || store.createLocale != "tr" {
			t.Fatalf("unexpected create args email=%q locale=%q", store.createEmail, store.createLocale)
		}
		if store.createPasswordHash != "bcrypt-hash" {
			t.Fatalf("expected provided hash to pass through, got %q", store.createPasswordHash)
		}
		if store.assignRoleUserID != "u2" || store.assignRoleName != "admin" {
			t.Fatalf("assign role call mismatch user=%q role=%q", store.assignRoleUserID, store.assignRoleName)
		}
	})

	t.Run("concurrent create returns email taken", func(t *testing.T) {
		store := &fakeStore{findByEmailErr: ErrNotFound, createErr: ErrEmailTaken}
		svc := NewService(store)

		if err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash"); err != nil {
			t.Fatalf("SeedAdmin should ignore concurrent email taken, got %v", err)
		}
	})

	t.Run("unexpected find error returns", func(t *testing.T) {
		store := &fakeStore{findByEmailErr: errors.New("boom")}
		svc := NewService(store)

		err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash")
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected boom error, got %v", err)
		}
	})

	t.Run("assign role failure for existing user returns error", func(t *testing.T) {
		store := &fakeStore{findByEmailUser: &User{ID: "u1"}, assignRoleErr: errors.New("assign failed")}
		svc := NewService(store)

		err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash")
		if err == nil || err.Error() != "assign failed" {
			t.Fatalf("expected assign failed error, got %v", err)
		}
	})

	t.Run("create failure other than email taken returns error", func(t *testing.T) {
		store := &fakeStore{findByEmailErr: ErrNotFound, createErr: errors.New("create failed")}
		svc := NewService(store)

		err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash")
		if err == nil || err.Error() != "create failed" {
			t.Fatalf("expected create failed error, got %v", err)
		}
	})

	t.Run("assign role failure after create returns error", func(t *testing.T) {
		store := &fakeStore{
			findByEmailErr: ErrNotFound,
			createUser:     &User{ID: "u2"},
			assignRoleErr:  errors.New("assign failed"),
		}
		svc := NewService(store)

		err := svc.SeedAdmin(context.TODO(), "admin@example.com", "bcrypt-hash")
		if err == nil || err.Error() != "assign failed" {
			t.Fatalf("expected assign failed error, got %v", err)
		}
	})
}

func TestServicePassthroughErrors(t *testing.T) {
	t.Run("register propagates repository error", func(t *testing.T) {
		store := &fakeStore{createErr: errors.New("repo down")}
		svc := NewService(store)

		_, err := svc.Register(context.TODO(), "user@example.com", "Password1!", "en")
		if err == nil || err.Error() != "repo down" {
			t.Fatalf("expected repo down error, got %v", err)
		}
	})

	t.Run("register with roles propagates repository error", func(t *testing.T) {
		store := &fakeStore{createWithRolesErr: errors.New("role assign failed")}
		svc := NewService(store)

		_, err := svc.RegisterWithRoles(context.TODO(), "user@example.com", "Password1!", "en", []string{"admin"})
		if err == nil || err.Error() != "role assign failed" {
			t.Fatalf("expected role assign failed error, got %v", err)
		}
	})

	t.Run("setters and updaters propagate errors", func(t *testing.T) {
		store := &fakeStore{
			setRolesErr:       errors.New("set roles failed"),
			setActiveErr:      errors.New("set active failed"),
			updatePasswordErr: errors.New("update password failed"),
			updateAvatarErr:   errors.New("update avatar failed"),
			permissionsErr:    errors.New("permissions failed"),
			listErr:           errors.New("list failed"),
			findByIDErr:       errors.New("find by id failed"),
			findByEmailErr:    errors.New("find by email failed"),
			updateProfileErr:  errors.New("update profile failed"),
		}
		svc := NewService(store)

		if _, err := svc.List(context.TODO(), ListParams{}); err == nil || err.Error() != "list failed" {
			t.Fatalf("expected list failed error, got %v", err)
		}
		if _, err := svc.FindByID(context.TODO(), "u1"); err == nil || err.Error() != "find by id failed" {
			t.Fatalf("expected find by id failed error, got %v", err)
		}
		if _, err := svc.FindByEmail(context.TODO(), "u1@example.com"); err == nil || err.Error() != "find by email failed" {
			t.Fatalf("expected find by email failed error, got %v", err)
		}
		if err := svc.SetRoles(context.TODO(), "u1", []string{"admin"}); err == nil || err.Error() != "set roles failed" {
			t.Fatalf("expected set roles failed error, got %v", err)
		}
		if err := svc.SetActive(context.TODO(), "u1", false); err == nil || err.Error() != "set active failed" {
			t.Fatalf("expected set active failed error, got %v", err)
		}
		if err := svc.UpdatePassword(context.TODO(), "u1", "hash"); err == nil || err.Error() != "update password failed" {
			t.Fatalf("expected update password failed error, got %v", err)
		}
		if _, err := svc.UpdateAvatar(context.TODO(), "u1", "key", 1); err == nil || err.Error() != "update avatar failed" {
			t.Fatalf("expected update avatar failed error, got %v", err)
		}
		if _, err := svc.GetPermissions(context.TODO(), "u1"); err == nil || err.Error() != "permissions failed" {
			t.Fatalf("expected permissions failed error, got %v", err)
		}
		if _, err := svc.UpdateProfile(context.TODO(), "u1", "Jane", "Doe"); err == nil || err.Error() != "update profile failed" {
			t.Fatalf("expected update profile failed error, got %v", err)
		}
	})
}
