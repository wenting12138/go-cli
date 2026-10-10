package skills

import "testing"

func TestDiscoverMatchesRelevantSkill(t *testing.T) {
	t.Parallel()

	skills := []Skill{
		{
			Name:          "java-maven",
			Description:   "Build and debug Maven Java projects",
			WhenToUse:     "Use for pom.xml, mvn compile, Java dependency issues",
			Content:       "Helpful for Maven repositories.",
			UserInvocable: true,
		},
		{
			Name:          "frontend-react",
			Description:   "Work on React UI code",
			WhenToUse:     "Use for components and CSS",
			Content:       "Helpful for React repositories.",
			UserInvocable: true,
		},
	}

	skill, ok := Discover(skills, "帮我看一下 pom.xml 为什么 mvn compile 失败")
	if !ok {
		t.Fatalf("expected a discovered skill")
	}
	if skill.Name != "java-maven" {
		t.Fatalf("expected java-maven, got %q", skill.Name)
	}
}

func TestDiscoverSkipsLowConfidenceQueries(t *testing.T) {
	t.Parallel()

	skills := []Skill{
		{
			Name:          "java-maven",
			Description:   "Build and debug Maven Java projects",
			WhenToUse:     "Use for pom.xml, mvn compile, Java dependency issues",
			UserInvocable: true,
		},
	}

	if _, ok := Discover(skills, "你好"); ok {
		t.Fatalf("expected no discovered skill for low-signal query")
	}
}

func TestDiscoverSkipsNonInvocableSkill(t *testing.T) {
	t.Parallel()

	skills := []Skill{
		{
			Name:          "secret-admin",
			Description:   "Admin-only skill",
			WhenToUse:     "Use for admin work",
			UserInvocable: false,
		},
		{
			Name:          "docs",
			Description:   "Docs skill",
			WhenToUse:     "Use for documentation",
			UserInvocable: true,
		},
	}

	if skill, ok := Discover(skills, "admin work"); ok && skill.Name == "secret-admin" {
		t.Fatalf("expected non-invocable skill to be ignored")
	}
}
