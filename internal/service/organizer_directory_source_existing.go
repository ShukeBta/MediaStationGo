package service

import (
	"context"
	"os"

	"go.uber.org/zap"
)

func skipAlreadyOrganizedSource(req organizeSourceFileRequest, plan organizeSourceFilePlan) {
	req.Result.Skipped++
	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: req.Source, Target: plan.Target.Path, Action: "skip", Reason: organizeSkipAlreadyOrganized,
		MediaType: plan.Layout.MediaType, Category: plan.Layout.Category, Title: plan.Identity.Title,
	})
}

func (o *OrganizerService) collectExistingSourceVersions(ctx context.Context, req organizeSourceFileRequest, plan organizeSourceFilePlan) organizeExistingSourceVersions {
	externalExisting := o.existingByExternalIdentity(ctx, req.DestRoot, plan.MetadataMatch, plan.Identity.Season, plan.Identity.Episode)
	identityExisting := o.existingByIdentity(ctx, req.DestRoot, plan.Identity.ParsedTitle, plan.Identity.Year, plan.Identity.Season, plan.Identity.Episode)
	folderExisting := o.existingByFolder(plan.Target.Dir, plan.Target.EpisodeTag)
	return organizeExistingSourceVersions{
		External: externalExisting,
		Identity: identityExisting,
		Folder:   folderExisting,
		All:      mergeExistingVersionPaths(externalExisting, identityExisting, folderExisting),
	}
}

func (o *OrganizerService) handleExistingSourceVersions(ctx context.Context, req organizeSourceFileRequest, plan organizeSourceFilePlan, existing organizeExistingSourceVersions) error {
	reclassified, err := o.reclassifyExistingMedia(ctx, organizeExistingReclassifyRequest{
		Source:          req.Source,
		Target:          plan.Target.Path,
		DestRoot:        req.DestRoot,
		TargetLibraryID: plan.TargetLibraryID,
		Existing:        existing.All,
		DryRun:          req.DryRun,
		MediaType:       plan.Layout.MediaType,
		Category:        plan.Layout.Category,
		Title:           plan.Identity.Title,
		Year:            plan.Identity.Year,
		Season:          plan.Identity.Season,
		Episode:         plan.Identity.Episode,
		MetadataMatch:   plan.MetadataMatch,
		Result:          req.Result,
	})
	if err != nil || reclassified {
		return err
	}
	if replaced, err := o.replaceSourceWithBetterVersion(ctx, req, plan, existing.All); replaced || err != nil {
		return err
	}
	o.skipExistingSourceDuplicate(ctx, req, plan, existing)
	return nil
}

func (o *OrganizerService) replaceSourceWithBetterVersion(ctx context.Context, req organizeSourceFileRequest, plan organizeSourceFilePlan, existing []string) (bool, error) {
	srcArea := o.resolutionArea(ctx, req.Source)
	bestArea := 0
	for _, e := range existing {
		if a := o.resolutionArea(ctx, e); a > bestArea {
			bestArea = a
		}
	}
	if !req.AllowReplaceExisting || srcArea <= 0 || bestArea <= 0 || srcArea <= bestArea {
		return false, nil
	}
	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: req.Source, Target: plan.Target.Path, Action: "replace", Reason: "higher resolution",
		MediaType: plan.Layout.MediaType, Category: plan.Layout.Category, Title: plan.Identity.Title,
	})
	if req.DryRun {
		req.Result.Replaced++
		return true, nil
	}
	if err := o.replaceVersions(ctx, req.Source, existing, plan.Target.Path, req.Mode); err != nil {
		return true, err
	}
	o.log.Info("organize replaced lower-resolution media",
		zap.String("from", req.Source),
		zap.String("to", plan.Target.Path),
		zap.Int("src_area", srcArea),
		zap.Int("existing_area", bestArea),
	)
	req.Result.Replaced++
	return true, nil
}

func (o *OrganizerService) skipExistingSourceDuplicate(ctx context.Context, req organizeSourceFileRequest, plan organizeSourceFilePlan, existing organizeExistingSourceVersions) {
	reason := organizeSkipTargetExists
	if len(existing.External) > 0 || len(existing.Identity) > 0 || o.allExistingPathsInDB(ctx, existing.All) {
		reason = organizeSkipDuplicateLibrary
	}
	o.log.Debug("organize skip duplicate",
		zap.String("src", req.Source), zap.String("dest_dir", plan.Target.Dir), zap.String("reason", reason))
	req.Result.Skipped++
	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: req.Source, Target: plan.Target.Path, Action: "skip", Reason: reason,
		MediaType: plan.Layout.MediaType, Category: plan.Layout.Category, Title: plan.Identity.Title,
	})
}

func (o *OrganizerService) writeOrganizedSourceFile(ctx context.Context, req organizeSourceFileRequest, plan organizeSourceFilePlan) error {
	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: req.Source, Target: plan.Target.Path, Action: "organize",
		MediaType: plan.Layout.MediaType, Category: plan.Layout.Category, Title: plan.Identity.Title,
	})
	if req.DryRun {
		req.Result.Organized++
		return nil
	}
	if err := os.MkdirAll(plan.Target.Dir, 0o755); err != nil { // #nosec G301 -- organized media directories must remain readable by NAS/player users.
		return err
	}
	if _, err := os.Stat(plan.Target.Path); err == nil {
		req.Result.Skipped++
		if len(req.Result.Items) > 0 {
			req.Result.Items[len(req.Result.Items)-1].Action = "skip"
			req.Result.Items[len(req.Result.Items)-1].Reason = organizeSkipTargetExists
		}
		return nil
	}
	if err := transferFile(req.Source, plan.Target.Path, req.Mode); err != nil {
		return err
	}
	if err := transferSidecarNFO(req.Source, plan.Target.Path, req.Mode); err != nil {
		o.log.Warn("organize sidecar nfo failed",
			zap.String("from", req.Source), zap.String("to", plan.Target.Path), zap.Error(err))
	}
	o.persistOrganizedSourceMetadata(ctx, plan)
	req.Result.Organized++
	return nil
}
