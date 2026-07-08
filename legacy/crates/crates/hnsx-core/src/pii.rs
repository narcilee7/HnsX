//! PII / secrets detector.
//!
//! Phase 2.7 adds row-level detection. For now this is a simple regex-based
//! scan for common sensitive patterns. Detected content causes the workflow
//! engine to emit a `Chunk::error` and short-circuit the stream.

use std::sync::OnceLock;

use regex::Regex;

/// Patterns considered sensitive. Additive only; keep false-positive rate low.
fn patterns() -> &'static [Regex] {
    static PATTERNS: OnceLock<Vec<Regex>> = OnceLock::new();
    PATTERNS.get_or_init(|| {
        vec![
            // Email addresses.
            Regex::new(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}").expect("email regex"),
            // US phone numbers (with or without separators).
            Regex::new(r"\b\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b").expect("phone regex"),
            // US Social Security Numbers.
            Regex::new(r"\b\d{3}-\d{2}-\d{4}\b").expect("ssn regex"),
            // Credit cards (Visa/Mastercard/Amex style, with or without spaces).
            Regex::new(r"\b(?:\d{4}[-\s]?){3}\d{4}\b").expect("cc regex"),
            // AWS access key id.
            Regex::new(r"\bAKIA[0-9A-Z]{16}\b").expect("aws key regex"),
        ]
    })
}

/// Returns the first matching PII pattern, if any.
pub fn detect(text: &str) -> Option<String> {
    for re in patterns() {
        if re.is_match(text) {
            return Some(re.as_str().to_string());
        }
    }
    None
}

/// Returns true if the text contains any known PII pattern.
pub fn contains_pii(text: &str) -> bool {
    detect(text).is_some()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_email() {
        assert!(contains_pii("contact me at alice@example.com please"));
    }

    #[test]
    fn detects_phone() {
        assert!(contains_pii("call 415-555-1234"));
        assert!(contains_pii("call (415) 555-1234"));
    }

    #[test]
    fn detects_ssn() {
        assert!(contains_pii("ssn 123-45-6789"));
    }

    #[test]
    fn detects_credit_card() {
        assert!(contains_pii("card 4111-1111-1111-1111"));
    }

    #[test]
    fn detects_aws_key() {
        assert!(contains_pii("AKIAIOSFODNN7EXAMPLE"));
    }

    #[test]
    fn clean_text_is_safe() {
        assert!(!contains_pii(
            "The quick brown fox jumps over the lazy dog."
        ));
    }
}
