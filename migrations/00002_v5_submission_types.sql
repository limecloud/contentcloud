ALTER TABLE submissions
  DROP CONSTRAINT submissions_submission_type_check,
  ADD CONSTRAINT submissions_submission_type_check
    CHECK (submission_type IN ('context','knowledge','strategy','offer','brief','content_batch','asset_batch','storyboard','delivery','result'));

ALTER TABLE approved_snapshots
  DROP CONSTRAINT approved_snapshots_submission_type_check,
  ADD CONSTRAINT approved_snapshots_submission_type_check
    CHECK (submission_type IN ('context','knowledge','strategy','offer','brief','content_batch','asset_batch','storyboard','delivery','result'));

-- +goose Down

ALTER TABLE approved_snapshots
  DROP CONSTRAINT approved_snapshots_submission_type_check,
  ADD CONSTRAINT approved_snapshots_submission_type_check
    CHECK (submission_type IN ('context','knowledge','brief','content_batch','asset_batch','delivery','result'));

ALTER TABLE submissions
  DROP CONSTRAINT submissions_submission_type_check,
  ADD CONSTRAINT submissions_submission_type_check
    CHECK (submission_type IN ('context','knowledge','brief','content_batch','asset_batch','delivery','result'));
