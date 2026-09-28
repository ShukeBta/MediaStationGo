package service

import "context"

func (s *MediaService) GetSTRMTarget(ctx context.Context, id string) (string, error) {
	media, err := s.repo.Media.FindByID(ctx, id)
	if err != nil {
		return "", err
	}
	if media == nil || !isLocalSTRMFile(media.Path) {
		return "", ErrMediaNotFound
	}
	return readLocalSTRMTarget(media.Path)
}
