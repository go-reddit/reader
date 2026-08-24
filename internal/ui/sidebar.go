package ui

// The sidebar's scrollable middle — the active profile's FEEDS list — is a
// toolkit.TreeView (s.sideTree), so the widget owns row layout, the scroll
// window and hit-testing while the reader supplies each row's label through its
// RowRenderer. The tree's Root is a synthetic hidden container (HideRoot) whose
// children are the flush, depth-0 feed rows; selecting one maps its TreeNode.Data
// (a feedNode identity) back to the HitFeed action. The profile band above and
// the pinned Account/Settings rows below are not part of the tree.

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"

	"github.com/go-reddit/reader/internal/settings"
)

// feedNode is the identity a FEEDS TreeNode carries in its Data, mapping a
// clicked row back to its subreddit ("" = the front page).
type feedNode struct {
	feed string
}

// feedNodeData returns a node's feedNode identity.
func feedNodeData(n *toolkit.TreeNode) feedNode { d, _ := n.Data.(feedNode); return d }

// profileNames is the profile tab labels, in order.
func profileNames(profiles []settings.Profile) []string {
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names
}

// buildSideTree (re)builds the FEEDS TreeView's node set from the active
// profile's subreddits and marks the row matching the current subreddit as
// Selected. It runs each layout: a handful of nodes, cheap, and keeps the row
// set + selection in lock-step with the model.
func (s *Scene) buildSideTree() {
	root := &toolkit.TreeNode{}
	var selected *toolkit.TreeNode
	for _, f := range s.ActiveFeeds() {
		n := &toolkit.TreeNode{Data: feedNode{feed: f}}
		if f == s.Subreddit {
			selected = n
		}
		root.Children = append(root.Children, n)
	}
	s.sideTree.Root = root
	s.sideTree.Selected().Set(selected)
}

// drawSideRow is the FEEDS TreeView RowRenderer: it paints one feed row's label
// in the content rect the widget hands it (already past the chevron slot), in
// the resolved ink (accent-contrast on the selected row's accent fill, else the
// theme's ink) — vertically centred to match the reader's other sidebar rows.
func (s *Scene) drawSideRow(p painter.Painter, th *toolkit.Theme, cr toolkit.Rect, node *toolkit.TreeNode, _ bool, ink toolkit.RGBA) {
	m := s.m
	label := "Front page"
	if f := feedNodeData(node).feed; f != "" {
		label = "r/" + f
	}
	top := cr.Y + (cr.H-m.side.height)/2
	m.side.labelAt(p, th, cr.X, top, label, ink)
}
