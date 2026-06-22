package auth

import (
	"testing"
)

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

func TestDeleteUser_RefusesLastAdminEvenWithMultiAdmin(t *testing.T) {
	m := newUserMgr(t)
	admin1, err := m.CreateUser("admin1", "pass1", "admin")
	if err != nil {
		t.Fatalf("create admin1: %v", err)
	}
	admin2, err := m.CreateUser("admin2", "pass2", "admin")
	if err != nil {
		t.Fatalf("create admin2: %v", err)
	}
	// Deleting admin1 should work — admin2 still exists
	if err := m.DeleteUser(admin1.ID); err != nil {
		t.Fatalf("delete admin1: %v", err)
	}
	// Deleting admin2 (last admin) should be refused
	if err := m.DeleteUser(admin2.ID); err == nil {
		t.Fatal("REGRESSION: deleting the last remaining admin must be refused")
	}
}
