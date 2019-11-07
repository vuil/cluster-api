package repository

import (
	"reflect"
	"testing"
)

func Test_execGoTemplates(t *testing.T) {
	type args struct {
		yaml    []byte
		options TemplateOptions
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "go template are executed",
			args: args{
				yaml: []byte("test {{ .ClusterName }}"),
				options: TemplateOptions{
					ClusterName: "test",
				},
			},
			want:    []byte("test test"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := execGoTemplates(tt.args.yaml, tt.args.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("execGoTemplates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("execGoTemplates() got = %v, want %v", got, tt.want)
			}
		})
	}
}
