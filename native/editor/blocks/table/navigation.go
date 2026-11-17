package table

func (c *Controller) ResetNavigation() { c.navigation = nil }

func (c *Controller) NavigationLen() int { return len(c.navigation) }

func (c *Controller) Replay(current Snapshot, rowDelta, colDelta int) (Snapshot, bool) {
	if len(c.navigation) == 0 {
		return Snapshot{}, false
	}
	step := c.navigation[len(c.navigation)-1]
	if !Equal(current, step.after) || rowDelta != -step.rowDelta || colDelta != -step.colDelta {
		if !Equal(current, step.after) {
			c.navigation = nil
		}
		return Snapshot{}, false
	}
	c.navigation = c.navigation[:len(c.navigation)-1]
	return Clone(step.before), true
}

func (c *Controller) Record(before, after Snapshot, rowDelta, colDelta int) {
	if Equal(before, after) {
		return
	}
	if before.Selection == nil && before.DocumentRegion == nil && before.Bridge == nil &&
		before.VirtualEdit == nil && before.Visual == nil &&
		after.Selection == nil && after.DocumentRegion == nil && after.Bridge == nil &&
		after.VirtualEdit == nil && after.Visual == nil &&
		len(c.navigation) == 0 {
		return
	}
	c.navigation = append(c.navigation, navigationStep{
		before: Clone(before), after: Clone(after), rowDelta: rowDelta, colDelta: colDelta,
	})
}

func Clone(s Snapshot) Snapshot {
	if s.Selection != nil {
		v := *s.Selection
		s.Selection = &v
	}
	if s.DocumentRegion != nil {
		v := *s.DocumentRegion
		s.DocumentRegion = &v
	}
	if s.Bridge != nil {
		v := *s.Bridge
		s.Bridge = &v
	}
	if s.VirtualEdit != nil {
		v := *s.VirtualEdit
		s.VirtualEdit = &v
	}
	if s.Visual != nil {
		v := *s.Visual
		s.Visual = &v
	}
	return s
}

func Equal(a, b Snapshot) bool {
	return a.Mode == b.Mode && a.TextSelection == b.TextSelection &&
		equalSelection(a.Selection, b.Selection) &&
		equalSelection(a.DocumentRegion, b.DocumentRegion) &&
		equalBridge(a.Bridge, b.Bridge) &&
		equalVirtualEdit(a.VirtualEdit, b.VirtualEdit) &&
		equalVisual(a.Visual, b.Visual)
}

func equalSelection(a, b *Selection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalBridge(a, b *ContentBridge) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalVirtualEdit(a, b *VirtualEdit) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalVisual(a, b *VisualSelection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
