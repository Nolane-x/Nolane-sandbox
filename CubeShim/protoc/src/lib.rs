// Copyright (c) 2020 Ant Financial
//
// SPDX-License-Identifier: Apache-2.0
//
#![allow(bare_trait_objects)]
#![allow(clippy::redundant_field_names)]
pub mod agent;
pub mod agent_ttrpc;
pub mod csi;
pub mod empty;
pub mod health;
pub mod health_ttrpc;
pub mod oci;
pub mod types;

// Keep serialization of generated agent protocol messages inside this crate.
// The generated agent types use protobuf 2.x, while CubeShim itself also uses
// protobuf 3.x through containerd-shim-protos. An inherent method prevents the
// caller from accidentally importing the wrong major-version Message trait.
impl agent::GetOOMVictimEvidenceResponse {
    pub fn write_to_bytes(&self) -> protobuf::ProtobufResult<Vec<u8>> {
        <Self as protobuf::Message>::write_to_bytes(self)
    }
}
