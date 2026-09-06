// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cube

// GuestKernelOOMVictimMarked reports only positive proof. Absence is UNKNOWN,
// never a negative claim about guest-kernel OOM victimization.
func (e TaskTerminationEvidence) GuestKernelOOMVictimMarked() (marked bool, known bool) {
	if len(e.GuestKernelOOMVictimProofs()) == 0 {
		return false, false
	}
	return true, true
}

// GuestMainKernelOOMVictimMarked reports whether one accepted Wave21 record is
// an exact MAIN lifetime match. No MAIN record means UNKNOWN, not false-known.
func (e TaskTerminationEvidence) GuestMainKernelOOMVictimMarked() (marked bool, known bool) {
	for _, proof := range e.GuestKernelOOMVictimProofs() {
		if proof.VictimClass == GuestKernelOOMVictimMain {
			return true, true
		}
	}
	return false, false
}
