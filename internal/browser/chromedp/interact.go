package chromedp

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/user/miniweb/internal/minidom"
)

// ClickElement finds element_id in snap and dispatches a click at its center.
func ClickElement(ctx context.Context, elementID int, snap *minidom.PageSnapshot) error {
	node := findByElementID(snap, elementID)
	if node == nil {
		return fmt.Errorf("element_id %d not found in snapshot", elementID)
	}
	if node.Layout == nil {
		return fmt.Errorf("element_id %d has no layout information", elementID)
	}

	// For links, use the href to navigate directly — more reliable than coordinate click.
	if node.Type == minidom.NodeLink && node.Interaction != nil && node.Interaction.Href != "" {
		return chromedp.Run(ctx,
			chromedp.Navigate(node.Interaction.Href),
			chromedp.WaitReady("body", chromedp.ByQuery),
		)
	}

	x := node.Layout.X + node.Layout.W/2
	y := node.Layout.Y + node.Layout.H/2
	return chromedp.Run(ctx, chromedp.MouseClickXY(x, y))
}

// SetInputValue sets an input/textarea/select value by element_id.
func SetInputValue(ctx context.Context, elementID int, value string, snap *minidom.PageSnapshot) error {
	node := findByElementID(snap, elementID)
	if node == nil {
		return fmt.Errorf("element_id %d not found in snapshot", elementID)
	}
	if node.Interaction == nil {
		return fmt.Errorf("element_id %d is not interactive", elementID)
	}

	// Build a JS selector by walking the bounding box to find the element.
	// Simpler: use evaluateSetValue which finds by position + sets value.
	x := 0.0
	y := 0.0
	if node.Layout != nil {
		x = node.Layout.X + node.Layout.W/2
		y = node.Layout.Y + node.Layout.H/2
	}

	js := fmt.Sprintf(`(function(){
		var el = document.elementFromPoint(%f, %f);
		if(el){el.value=%q; el.dispatchEvent(new Event('input',{bubbles:true})); el.dispatchEvent(new Event('change',{bubbles:true})); return true;}
		return false;
	})()`, x-0 /* adjust for scroll */, y-0, value)

	var ok bool
	return chromedp.Run(ctx, chromedp.Evaluate(js, &ok))
}

// SubmitForm submits the form identified by formElementID, first filling fields.
func SubmitForm(ctx context.Context, formElementID int, fieldValues map[string]string, snap *minidom.PageSnapshot) error {
	// Fill each field.
	for _, node := range snap.Nodes {
		if node.Interaction == nil {
			continue
		}
		if node.Interaction.FormID != formElementID {
			continue
		}
		name := node.Interaction.Name
		val, ok := fieldValues[name]
		if !ok {
			continue
		}
		if err := SetInputValue(ctx, node.Interaction.ElementID, val, snap); err != nil {
			return fmt.Errorf("fill field %q: %w", name, err)
		}
	}

	// Submit the form.
	formNode := findByElementID(snap, formElementID)
	if formNode == nil {
		return fmt.Errorf("form element_id %d not found", formElementID)
	}

	x := 0.0
	y := 0.0
	if formNode.Layout != nil {
		x = formNode.Layout.X + formNode.Layout.W/2
		y = formNode.Layout.Y + formNode.Layout.H/2
	}

	js := fmt.Sprintf(`(function(){
		var el = document.elementFromPoint(%f, %f);
		while(el && el.tagName && el.tagName.toLowerCase() !== 'form') el = el.parentElement;
		if(el && el.tagName && el.tagName.toLowerCase() === 'form'){el.submit(); return true;}
		return false;
	})()`, x, y)

	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
		return err
	}
	// Wait for navigation after submit.
	return chromedp.Run(ctx, chromedp.WaitReady("body", chromedp.ByQuery))
}

func findByElementID(snap *minidom.PageSnapshot, elementID int) *minidom.Node {
	for i := range snap.Nodes {
		if snap.Nodes[i].Interaction != nil && snap.Nodes[i].Interaction.ElementID == elementID {
			return &snap.Nodes[i]
		}
	}
	return nil
}
