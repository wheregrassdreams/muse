package markdown

import "muse/markdown/images"

func fetchRemoteImageData(src string, store images.CacheStore) ([]byte, error) {
	return images.NewRemoteCache(store, images.ValidateImageData).Fetch(src)
}
