package auth

import (
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

// newUserMgr builds a UserManager backed by a fresh in-memory DB with the full
// schema (so the users table exists exactly as in production).
func newUserMgr(t *testing.T) *UserManager {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewUserManager(database.Conn(), "test-secret")
}

func TestDeleteUser_RemovesNonAdmin(t *testing.T) {
	m := newUserMgr(t)
	if _, err := m.CreateUser("admin", "adminpass123", "admin"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	u, err := m.CreateUser("bob", "bobpass123", "user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := m.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := m.GetByID(u.ID); err == nil {
		t.Fatal("expected user to be gone after delete")
	}
}

func TestDeleteUser_RefusesLastAdmin(t *testing.T) {
	m := newUserMgr(t)
	admin, err := m.CreateUser("admin", "adminpass123", "admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := m.DeleteUser(admin.ID); err == nil {
		t.Fatal("REGRESSION: deleting the last admin must be refused")
	}
	if _, err := m.GetByID(admin.ID); err != nil {
		t.Fatal("last admin must still exist after refused delete")
	}
}

func TestDeleteUser_AllowsAdminWhenAnotherExists(t *testing.T) {
	m := newUserMgr(t)
	a1, _ := m.CreateUser("admin1", "adminpass123", "admin")
	_, _ = m.CreateUser("admin2", "adminpass123", "admin")
	if err := m.DeleteUser(a1.ID); err != nil {
		t.Fatalf("should allow deleting an admin when another admin exists: %v", err)
	}
}

func TestUpdateUserRole_PromoteAndDemote(t *testing.T) {
	m := newUserMgr(t)
	_, _ = m.CreateUser("admin", "adminpass123", "admin")
	bob, _ := m.CreateUser("bob", "bobpass123", "user")
	if err := m.UpdateUserRole(bob.ID, "admin"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	got, _ := m.GetByID(bob.ID)
	if got.Role != "admin" {
		t.Fatalf("expected role admin, got %q", got.Role)
	}
	// Now bob is admin too, so demoting the original admin is allowed.
	if err := m.UpdateUserRole(bob.ID, "user"); err != nil {
		t.Fatalf("demote: %v", err)
	}
}

func TestUpdateUserRole_RefusesDemoteLastAdmin(t *testing.T) {
	m := newUserMgr(t)
	admin, _ := m.CreateUser("admin", "adminpass123", "admin")
	if err := m.UpdateUserRole(admin.ID, "user"); err == nil {
		t.Fatal("REGRESSION: demoting the last admin must be refused")
	}
}

func TestUpdateUserRole_RejectsInvalidRole(t *testing.T) {
	m := newUserMgr(t)
	bob, _ := m.CreateUser("bob", "bobpass123", "user")
	if err := m.UpdateUserRole(bob.ID, "superuser"); err == nil {
		t.Fatal("expected invalid role to be rejected")
	}
}

func TestAdminSetPassword_ResetsAndFlagsRotation(t *testing.T) {
	m := newUserMgr(t)
	bob, _ := m.CreateUser("bob", "bobpass123", "user")
	if err := m.AdminSetPassword(bob.ID, "newpass456"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	// New password authenticates.
	if _, _, err := m.Authenticate("bob", "newpass456"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
	// Old password rejected.
	if _, _, err := m.Authenticate("bob", "bobpass123"); err == nil {
		t.Fatal("old password must no longer authenticate")
	}
	// must_change_password flag set.
	got, _ := m.GetByID(bob.ID)
	if !got.MustChangePassword {
		t.Fatal("reset must flag must_change_password so the user rotates on next login")
	}
}

func TestAdminSetPassword_RejectsShortPassword(t *testing.T) {
	m := newUserMgr(t)
	bob, _ := m.CreateUser("bob", "bobpass123", "user")
	if err := m.AdminSetPassword(bob.ID, "short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
}

func TestAdminSetPassword_UnknownUser(t *testing.T) {
	m := newUserMgr(t)
	if err := m.AdminSetPassword("nonexistent", "validpass123"); err == nil {
		t.Fatal("expected error for unknown user")
	}
}

// TestListUsers_SurfacesMustChangeFlag guards a real bug caught in staging:
// ListUsers originally didn't SELECT must_change_password, so the flag always
// read false and the dashboard's "must change" badge never appeared even right
// after an admin password reset.
func TestListUsers_SurfacesMustChangeFlag(t *testing.T) {
	m := newUserMgr(t)
	bob, _ := m.CreateUser("bob", "bobpass123", "user")
	if err := m.AdminSetPassword(bob.ID, "resetpass456"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	list, err := m.ListUsers()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found bool
	for _, u := range list {
		if u.Username == "bob" {
			found = true
			if !u.MustChangePassword {
				t.Fatal("REGRESSION: ListUsers must surface must_change_password=true after a reset")
			}
		}
	}
	if !found {
		t.Fatal("bob missing from ListUsers")
	}
}
