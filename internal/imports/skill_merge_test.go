package imports

import (
	"context"
	"reflect"
	"testing"
)

func TestMergeSkillMergesEachFileOrListsItsConflicts(t *testing.T) {
	const text = "one\ntwo\nthree\nfour\nfive\n"
	copyOf := func(files map[string]string, executable ...string) SkillCopy {
		result := SkillCopy{Files: map[string][]byte{}, Executable: executable}
		for name, data := range files {
			result.Files[name] = []byte(data)
		}
		return result
	}
	base := copyOf(map[string]string{"SKILL.md": text, "run.sh": "run\n", "notes.md": "notes\n"})
	tests := map[string]struct {
		fork, upstream SkillCopy
		want           SkillCopy
		conflicts      []string
	}{
		"an unedited fork takes the source's copy": {
			fork:     base,
			upstream: copyOf(map[string]string{"SKILL.md": "zero\n" + text, "run.sh": "run\n", "new.md": "new\n"}, "run.sh"),
			want:     copyOf(map[string]string{"SKILL.md": "zero\n" + text, "run.sh": "run\n", "new.md": "new\n"}, "run.sh"),
		},
		"edits to other lines merge": {
			fork:     copyOf(map[string]string{"SKILL.md": "one\ntwo\nthree\nfour\nFIVE\n", "run.sh": "run\n", "notes.md": "notes\n"}),
			upstream: copyOf(map[string]string{"SKILL.md": "ONE\ntwo\nthree\nfour\nfive\n", "run.sh": "run\n", "notes.md": "notes\n"}),
			want:     copyOf(map[string]string{"SKILL.md": "ONE\ntwo\nthree\nfour\nFIVE\n", "run.sh": "run\n", "notes.md": "notes\n"}),
		},
		"the same edit on both sides merges": {
			fork:     copyOf(map[string]string{"SKILL.md": "ONE\n" + text[4:], "run.sh": "run\n"}),
			upstream: copyOf(map[string]string{"SKILL.md": "ONE\n" + text[4:], "run.sh": "run\n"}),
			want:     copyOf(map[string]string{"SKILL.md": "ONE\n" + text[4:], "run.sh": "run\n"}),
		},
		"edits to the same lines, a deleted file the other side changed, and a binary file conflict": {
			fork:      copyOf(map[string]string{"SKILL.md": "uno\n" + text[4:], "run.sh": "run fast\n", "logo.png": "\x00a"}),
			upstream:  copyOf(map[string]string{"SKILL.md": "eins\n" + text[4:], "notes.md": "more notes\n", "logo.png": "\x00b"}),
			conflicts: []string{"SKILL.md", "logo.png", "notes.md", "run.sh"},
		},
		"an executable bit one side sets and contents the other changes merge": {
			fork:     copyOf(map[string]string{"SKILL.md": text, "run.sh": "run\n", "notes.md": "notes\n"}, "run.sh"),
			upstream: copyOf(map[string]string{"SKILL.md": text, "run.sh": "run\nagain\n", "notes.md": "notes\n"}),
			want:     copyOf(map[string]string{"SKILL.md": text, "run.sh": "run\nagain\n", "notes.md": "notes\n"}, "run.sh"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			merged, conflicts, err := MergeSkill(context.Background(), Options{}, base, test.fork, test.upstream)
			if err != nil {
				t.Fatal(err)
			}
			if test.conflicts != nil {
				if !reflect.DeepEqual(conflicts, test.conflicts) || merged.Files != nil {
					t.Fatalf("conflicts = %v, merged %v; want only %v", conflicts, merged, test.conflicts)
				}
				return
			}
			if test.want.Executable == nil {
				test.want.Executable = []string{}
			}
			if len(conflicts) != 0 || !reflect.DeepEqual(merged, test.want) {
				t.Fatalf("merged = %q, conflicts %v; want %q", merged, conflicts, test.want)
			}
		})
	}
}
