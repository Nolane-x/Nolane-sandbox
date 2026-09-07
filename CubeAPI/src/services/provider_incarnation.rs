// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use uuid::Uuid;

    #[test]
    fn v25_provider_incarnation_mint_is_canonical_uuid_v4_and_non_repeating() {
        let first = new_provider_incarnation_id();
        let second = new_provider_incarnation_id();
        assert_ne!(first, second, "two create boundaries reused one incarnation ID");

        let parsed = Uuid::parse_str(&first).expect("minted provider incarnation is not a UUID");
        assert_eq!(parsed.get_version_num(), 4, "provider incarnation is not UUIDv4");
        assert_eq!(
            parsed.hyphenated().to_string(),
            first,
            "provider incarnation is not canonical lowercase hyphenated UUID text"
        );
    }

    #[test]
    fn v25_provider_incarnation_reader_accepts_only_canonical_uuid_v4() {
        let mut annotations = HashMap::new();
        assert_eq!(provider_incarnation_from_annotations(&annotations), None);

        annotations.insert(
            PROVIDER_INCARNATION_ANNOTATION.to_string(),
            "not-a-uuid".to_string(),
        );
        assert_eq!(provider_incarnation_from_annotations(&annotations), None);

        annotations.insert(
            PROVIDER_INCARNATION_ANNOTATION.to_string(),
            "550E8400-E29B-41D4-A716-446655440000".to_string(),
        );
        assert_eq!(provider_incarnation_from_annotations(&annotations), None);

        annotations.insert(
            PROVIDER_INCARNATION_ANNOTATION.to_string(),
            "550e8400-e29b-11d4-a716-446655440000".to_string(),
        );
        assert_eq!(provider_incarnation_from_annotations(&annotations), None);

        let canonical = "550e8400-e29b-41d4-a716-446655440000".to_string();
        annotations.insert(PROVIDER_INCARNATION_ANNOTATION.to_string(), canonical.clone());
        assert_eq!(
            provider_incarnation_from_annotations(&annotations),
            Some(canonical)
        );
    }
}
