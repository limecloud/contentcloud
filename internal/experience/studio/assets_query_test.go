package studio

import (
	"context"
	"testing"
	"time"
)

type workspaceAssetReaderStub struct {
	folders   []WorkspaceFolderRecord
	materials []WorkspaceMaterialRecord
	revisions map[string]SourceRevisionRecord
}

func (s workspaceAssetReaderStub) WorkspaceFolders(context.Context, string, string) ([]WorkspaceFolderRecord, error) {
	return s.folders, nil
}

func (s workspaceAssetReaderStub) WorkspaceMaterials(context.Context, string, string) ([]WorkspaceMaterialRecord, error) {
	return s.materials, nil
}

func (s workspaceAssetReaderStub) SourceRevision(_ context.Context, _, id string) (SourceRevisionRecord, error) {
	return s.revisions[id], nil
}

func TestAssetQueriesBuildsMaterialProjectionThroughNarrowPort(t *testing.T) {
	now := time.Date(2026, time.August, 8, 8, 0, 0, 0, time.UTC)
	queries := NewAssetQueries(workspaceAssetReaderStub{
		folders: []WorkspaceFolderRecord{{ID: "folder-1", ProjectID: "project-1", Name: "品牌资料"}},
		materials: []WorkspaceMaterialRecord{{
			ID: "material-1", ProjectID: "project-1", FolderID: "folder-1", SourceRevisionID: "revision-1",
			MaterialKind: "document", Origin: "uploaded", Usage: "project_material", Title: "品牌手册",
		}},
		revisions: map[string]SourceRevisionRecord{
			"revision-1": {FileName: "brand.md", DetectedMIME: "text/markdown", ProcessingStatus: "ready"},
		},
	}, func() time.Time { return now })

	projection, err := queries.WorkspaceMaterials(context.Background(), "tenant-1", "project-1", map[string]string{"project-1": "示例品牌"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Folders) != 1 || projection.Folders[0].Ref != "folder:folder-1" {
		t.Fatalf("unexpected folders: %#v", projection.Folders)
	}
	if len(projection.Materials) != 1 || projection.Materials[0].Ref != "material:material-1" {
		t.Fatalf("unexpected materials: %#v", projection.Materials)
	}
	if projection.Materials[0].ProcessingState != "ready" || projection.Counts["document"] != 1 {
		t.Fatalf("unexpected material projection: %#v", projection)
	}
}

func TestBuildAssetSurfaceKeepsOnlyEightRecentItemsPerProjection(t *testing.T) {
	base := time.Date(2026, time.August, 8, 8, 0, 0, 0, time.UTC)
	workspace := WorkspaceMaterialProjection{Materials: make([]WorkspaceMaterialItem, 9)}
	results := CreativeResultProjection{Items: make([]CreativeResultItem, 9)}
	for index := 0; index < 9; index++ {
		workspace.Materials[index] = WorkspaceMaterialItem{Ref: string(rune('a' + index)), UpdatedAt: base.Add(time.Duration(index) * time.Minute)}
		results.Items[index] = CreativeResultItem{Ref: string(rune('a' + index)), CreatedAt: base.Add(time.Duration(index) * time.Minute)}
	}
	surface := BuildAssetSurface(workspace, results, base)
	if len(surface.Recent.Materials) != 8 || surface.Recent.Materials[0].Ref != "i" {
		t.Fatalf("materials were not sorted and capped: %#v", surface.Recent.Materials)
	}
	if len(surface.Recent.Results) != 8 || surface.Recent.Results[0].Ref != "i" {
		t.Fatalf("results were not sorted and capped: %#v", surface.Recent.Results)
	}
}

func TestBuildCreativeResultProjectionSortsAndCountsReusableItems(t *testing.T) {
	base := time.Date(2026, time.August, 8, 8, 0, 0, 0, time.UTC)
	projection := BuildCreativeResultProjection([]CreativeResultItem{
		{Ref: "result:old", ResultType: "script", CreatedAt: base, Reusable: false},
		{Ref: "result:new", ResultType: "script", CreatedAt: base.Add(time.Minute), Reusable: true},
		{Ref: "result:image", ResultType: "image", CreatedAt: base.Add(2 * time.Minute), Reusable: true},
	}, base)
	if len(projection.Items) != 3 || projection.Items[0].Ref != "result:image" {
		t.Fatalf("results were not sorted: %#v", projection.Items)
	}
	if projection.Counts["all"] != 3 || projection.Counts["script"] != 2 || projection.Counts["reusable"] != 2 {
		t.Fatalf("unexpected result counts: %#v", projection.Counts)
	}
}
