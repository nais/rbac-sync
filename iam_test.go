package main

import (
	"k8s.io/api/core/v1"
	"testing"

	"google.golang.org/api/admin/directory/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	groupName = "test@test.com"
	ns1       = v1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:        "testspace",
		Annotations: map[string]string{"rbac-sync.nais.io/group-name": groupName},
	}}
)

func TestUniq(t *testing.T) {
	uniqUserList1 := uniq(getTestMembers())
	list1Length := len(uniqUserList1)
	if list1Length != 1 {
		t.Errorf("Uniq was incorrect, got: %d, want: %d.", list1Length, 1)
	}

	var uniqUserList2 []*admin.Member
	var member1 = new(admin.Member)
	member1.Email = "member1@example.com"
	var member2 = new(admin.Member)
	member2.Email = "member2@example.com"
	var member3 = new(admin.Member)
	member3.Email = "member3@example.com"
	uniqUserList2 = append(uniqUserList2, member1)
	uniqUserList2 = append(uniqUserList2, member2)
	uniqUserList2 = append(uniqUserList2, member1)
	uniqUserList2 = append(uniqUserList2, member3)
	uniqUserList2 = append(uniqUserList2, member2)
	uniqUserList2 = uniq(uniqUserList2)
	list2Length := len(uniqUserList2)
	if list2Length != 3 {
		t.Errorf("Uniq was incorrect, got: %d, want: %d.", list2Length, 3)
	}
	if uniqUserList2[0].Email != member1.Email {
		t.Errorf("Uniq sort was incorrect, got: %q, want: %q.", uniqUserList2[0].Email, member1.Email)
	}
	if uniqUserList2[1].Email != member2.Email {
		t.Errorf("Uniq sort was incorrect, got: %q, want: %q.", uniqUserList2[1].Email, member2.Email)
	}
	if uniqUserList2[2].Email != member3.Email {
		t.Errorf("Uniq sort was incorrect, got: %q, want: %q.", uniqUserList2[2].Email, member3.Email)
	}
}

// Build and returns a fake Admin members object.
func getTestMembers() []*admin.Member {
	var fakeResult []*admin.Member
	var fakeMember = new(admin.Member)
	fakeMember.Email = "faketeam@test.com"
	fakeResult = append(fakeResult, fakeMember)
	return fakeResult
}
