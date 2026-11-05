package service

import "testing"

func TestParseProbeJSONExtractsDOVIConfigurationFromPrimaryVideo(t *testing.T) {
	got, err := parseProbeJSON([]byte(`{
		"streams": [
			{"index":0,"codec_type":"video","codec_name":"hevc","side_data_list":[
				{"side_data_type":"DOVI configuration record","dv_profile":5,"dv_bl_signal_compatibility_id":0}
			]},
			{"index":1,"codec_type":"video","codec_name":"hevc","side_data_list":[
				{"side_data_type":"DOVI configuration record","dv_profile":8,"dv_bl_signal_compatibility_id":1}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("parseProbeJSON: %v", err)
	}
	if !got.HasDOVIConfiguration || !got.HasDVProfile || !got.HasDVBLCompatibilityID || got.DVProfile != 5 || got.DVBLCompatibilityID != 0 {
		t.Fatalf("DOVI fields = %+v, want primary stream profile 5 compat 0", got)
	}
}

func TestParseProbeJSONKeepsMissingDOVIValuesDistinctFromZero(t *testing.T) {
	got, err := parseProbeJSON([]byte(`{
		"streams": [{"index":0,"codec_type":"video","side_data_list":[
			{"side_data_type":"DOVI configuration record","dv_profile":5}
		]}]
	}`))
	if err != nil {
		t.Fatalf("parseProbeJSON: %v", err)
	}
	if !got.HasDOVIConfiguration || !got.HasDVProfile || got.HasDVBLCompatibilityID {
		t.Fatalf("missing DOVI compatibility value was not preserved: %+v", got)
	}
}

func TestParseProbeJSONDoesNotReadDOVIFromAttachedCoverOrAudio(t *testing.T) {
	got, err := parseProbeJSON([]byte(`{
		"streams": [
			{"index":0,"codec_type":"video","disposition":{"attached_pic":1},"side_data_list":[
				{"side_data_type":"DOVI configuration record","dv_profile":5,"dv_bl_signal_compatibility_id":0}
			]},
			{"index":1,"codec_type":"audio","side_data_list":[
				{"side_data_type":"DOVI configuration record","dv_profile":5,"dv_bl_signal_compatibility_id":0}
			]},
			{"index":2,"codec_type":"video","codec_name":"hevc"}
		]
	}`))
	if err != nil {
		t.Fatalf("parseProbeJSON: %v", err)
	}
	if got.HasDOVIConfiguration {
		t.Fatalf("non-primary video DOVI data was accepted: %+v", got)
	}
}
