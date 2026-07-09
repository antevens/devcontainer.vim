package devcontainer

import "testing"

func TestRemoteHomeFromUser(t *testing.T) {
	tests := []struct {
		user string
		want string
	}{
		{"root", "/root"},
		{"vscode", "/home/vscode"},
		{"ubuntu", "/home/ubuntu"},
		{"myuser", "/home/myuser"},
	}
	for _, tc := range tests {
		got := remoteHomeFromUser(tc.user)
		if got != tc.want {
			t.Errorf("remoteHomeFromUser(%q) = %q, want %q", tc.user, got, tc.want)
		}
	}
}

func TestGetRemoteHome(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "root user",
			input: `{"configuration":{"remoteUser":"root","configFilePath":{"fsPath":"/work/.devcontainer/devcontainer.json"}}}`,
			want:  "/root",
		},
		{
			name:  "vscode user",
			input: `{"configuration":{"remoteUser":"vscode","configFilePath":{"fsPath":"/work/.devcontainer/devcontainer.json"}}}`,
			want:  "/home/vscode",
		},
		{
			name:  "arbitrary user",
			input: `{"configuration":{"remoteUser":"ubuntu","configFilePath":{"fsPath":"/work/.devcontainer/devcontainer.json"}}}`,
			want:  "/home/ubuntu",
		},
		{
			name:  "no remoteUser set falls back to vscode default",
			input: `{"configuration":{"configFilePath":{"fsPath":"/work/.devcontainer/devcontainer.json"}}}`,
			want:  "/home/vscode",
		},
		{
			name:  "invalid JSON falls back to vscode default",
			input: `not json`,
			want:  "/home/vscode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetRemoteHome(tc.input)
			if got != tc.want {
				t.Errorf("GetRemoteHome() = %q, want %q", got, tc.want)
			}
		})
	}
}
