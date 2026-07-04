package service

import "testing"

func TestEvaluateFFmpegSecurityClassifiesPixelSmashFixedVersion(t *testing.T) {
	tests := []struct {
		name        string
		versionLine string
		want        string
	}{
		{
			name:        "ffmpeg 8.0 is vulnerable",
			versionLine: "ffmpeg version 8.0 Copyright",
			want:        "vulnerable",
		},
		{
			name:        "ffmpeg 8.1.1 is vulnerable",
			versionLine: "ffmpeg version 8.1.1 Copyright",
			want:        "vulnerable",
		},
		{
			name:        "ffmpeg 8.1.2 is ok",
			versionLine: "ffmpeg version 8.1.2 Copyright",
			want:        "ok",
		},
		{
			name:        "older major needs distro review",
			versionLine: "ffmpeg version 7.1.2 Copyright",
			want:        "review",
		},
		{
			name:        "unrecognized is unknown",
			versionLine: "not ffmpeg",
			want:        "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateFFmpegSecurity(tt.versionLine); got.Status != tt.want {
				t.Fatalf("status = %q, want %q (%+v)", got.Status, tt.want, got)
			}
		})
	}
}
