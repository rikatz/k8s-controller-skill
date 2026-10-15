// +kubebuilder:object:generate=true
// +groupName=cm.example.com
package cm

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TypeA has a doc comment.
//
// Deviation: type-level note after blank line, same group.
type TypeA struct {
	// A doc comment directly above.
	// +optional
	FA string `json:"fa,omitempty"`

	// Second paragraph test first paragraph.
	//
	// Second paragraph after blank slash line.
	// +optional
	FB string `json:"fb,omitempty"`

	// Hidden: separate group above (blank line between).

	// Visible: group directly above the field.
	// +optional
	FC string `json:"fc,omitempty"`

	// +optional
	FD string `json:"fd,omitempty"` // trailing eol comment

	// +notamarker this looks like a marker but is unknown
	// Text after unknown marker.
	// +optional
	FE string `json:"fe,omitempty"`

	// Normal first line.
	// Deviation: last line of the doc comment.
	// +optional
	FF string `json:"ff,omitempty"`

	// Normal first line.
	// +optional
	// Deviation: after the marker line.
	FG string `json:"fg,omitempty"`

	// Description for FH.
	// +optional
	//
	// Deviation: after marker and blank //.
	FH string `json:"fh,omitempty"`

	// Description for FI.
	//
	// +optional
	FI string `json:"fi,omitempty"`

	// Description for FJ.
	// +optional
	/* block comment Deviation */
	FJ string `json:"fj,omitempty"`

	// Description FK.
	// Deviation: before marker.
	// +kubebuilder:validation:MaxLength=5
	// +optional
	FK string `json:"fk,omitempty"`

	// Description FL.
	// TODO(alice): fix later
	// +optional
	FL string `json:"fl,omitempty"`

	// Hidden group with blank line gap.

	// +optional
	FM string `json:"fm,omitempty"`

	// Description FN.
	// nolint:revive // lint-style directive
	//lint:ignore ST1000 reason
	//go:generate echo hi
	// +optional
	FN string `json:"fn,omitempty"`
}

// TypeB has a doc.
// Deviation: last line of type doc.
type TypeB struct {
	// +optional
	X string `json:"x,omitempty"`
}

// TypeC has a doc.

// Deviation: separate group above type (blank line).
type TypeC struct {
	// +optional
	X string `json:"x,omitempty"`
}

// TypeD has a doc. // trailing
type TypeD struct {
	// +optional
	X string `json:"x,omitempty"`
}

// Deviation: HIDDEN detached group above type doc (blank line between).

// TypeE real doc.
type TypeE struct {
	// +optional
	X string `json:"x,omitempty"`
}

// Deviation: HIDDEN detached group above root type.

// CM is the root doc.
// +kubebuilder:object:root=true
type CM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	A                 TypeA `json:"a,omitzero"`
	B                 TypeB `json:"b,omitzero"`
	C                 TypeC `json:"c,omitzero"`
	D                 TypeD `json:"d,omitzero"`
	E                 TypeE `json:"e,omitzero"`
}

// +kubebuilder:object:root=true
type CMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CM `json:"items"`
}
