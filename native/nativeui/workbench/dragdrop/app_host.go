// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func treeItemTransferData(item dragpayload.VaultItem) (transfer.Data, error) {
	return treeItemsTransferData([]dragpayload.VaultItem{item})
}

func treeItemsTransferData(items []dragpayload.VaultItem) (transfer.Data, error) {
	if len(items) == 0 {
		return transfer.Data{}, fmt.Errorf("%s", locales.FilesNoDraggableItems())
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if !filepath.IsAbs(item.Path) {
			return transfer.Data{}, fmt.Errorf("%s", locales.FilesAbsolutePathRequired())
		}
		paths = append(paths, item.Path)
	}
	files, err := transfer.FileData(paths...)
	if err != nil {
		return transfer.Data{}, err
	}
	for i := range items {
		if items[i].Name == "" {
			items[i].Name = filepath.Base(items[i].Path)
		}
	}
	b, err := json.Marshal(struct {
		Path      string                  `json:"path"`
		Name      string                  `json:"name"`
		Directory bool                    `json:"directory"`
		Items     []dragpayload.VaultItem `json:"items,omitempty"`
	}{items[0].Path, items[0].Name, items[0].Directory, items})
	if err != nil {
		return transfer.Data{}, err
	}
	fileItems := files.Items()
	dataItems := make([]transfer.Item, 0, len(fileItems))
	for i, file := range fileItems {
		fileList, err := file.Read(transfer.FileList)
		if err != nil {
			return transfer.Data{}, err
		}
		uriList, err := file.Read(transfer.URIList)
		if err != nil {
			return transfer.Data{}, err
		}
		reps := []transfer.Representation{
			transfer.Bytes(transfer.FileList, fileList),
			transfer.Bytes(transfer.URIList, uriList),
			transfer.Bytes(transfer.Text, []byte(items[i].Path)),
		}
		if i == 0 {
			reps = append(reps, transfer.Bytes(transfer.Format("application/x-muse-vault-item+json"), b))
		}
		dataItems = append(dataItems, transfer.NewItem(reps...))
	}
	return transfer.New(dataItems...), nil
}
