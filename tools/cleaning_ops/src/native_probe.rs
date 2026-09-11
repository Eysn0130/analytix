use std::env;
use std::io::{self, Read, Write};
use std::thread;

pub(crate) const ARGUMENT: &str = "--analytix-native-probe";

const COMPONENT_ID: &str = "cleaning-ops";
const ENGINE_ID: &str = "analytix-cleaning-ops";
const PROTOCOL_VERSION: &str = "analytix-native-v1";
const LAUNCH_NONCE_ENV: &str = "ANALYTIX_NATIVE_LAUNCH_NONCE";
const PROTOCOL_VERSION_ENV: &str = "ANALYTIX_NATIVE_PROTOCOL_VERSION";
const PROTOCOL_ID_BYTES: usize = 64;
const REQUEST_PREFIX: &[u8] = b"{\"request_id\":\"";
const REQUEST_SUFFIX: &[u8] = b"\",\"command\":\"ping\"}\n";
const REQUEST_FRAME_BYTES: usize = REQUEST_PREFIX.len() + PROTOCOL_ID_BYTES + REQUEST_SUFFIX.len();

pub(crate) fn run() -> Result<(), ()> {
    let nonce = env::var(LAUNCH_NONCE_ENV);
    let protocol = env::var(PROTOCOL_VERSION_ENV);
    env::remove_var(LAUNCH_NONCE_ENV);
    env::remove_var(PROTOCOL_VERSION_ENV);
    let nonce = nonce.map_err(|_| ())?;
    let protocol = protocol.map_err(|_| ())?;
    if !is_protocol_id(nonce.as_bytes()) || protocol != PROTOCOL_VERSION {
        return Err(());
    }

    let process_id = std::process::id();
    let stdout = io::stdout();
    let mut output = stdout.lock();
    write!(
        output,
        "{{\"kind\":\"analytix_native_ready\",\"schema_version\":3,\"component_id\":\"{COMPONENT_ID}\",\"launch_nonce\":\"{nonce}\",\"protocol_version\":\"{PROTOCOL_VERSION}\",\"process_id\":{process_id}}}\n"
    )
    .map_err(|_| ())?;
    output.flush().map_err(|_| ())?;

    let stdin = io::stdin();
    let request_id = read_ping_request(&mut stdin.lock())?;
    let request_id = std::str::from_utf8(&request_id).map_err(|_| ())?;
    write!(
        output,
        "{{\"request_id\":\"{request_id}\",\"ok\":true,\"data\":{{\"pong\":true,\"pid\":{process_id}}},\"diagnostics\":{{\"engine\":\"{ENGINE_ID}\",\"command\":\"ping\",\"case_bound\":false,\"db_bound\":false,\"pid\":{process_id},\"queue_wait_ms\":0,\"run_ms\":0,\"owner_epoch\":\"\"}}}}\n"
    )
    .map_err(|_| ())?;
    output.flush().map_err(|_| ())?;

    wait_for_host_termination()
}

fn read_ping_request<R: Read>(reader: &mut R) -> Result<[u8; PROTOCOL_ID_BYTES], ()> {
    let mut frame = [0_u8; REQUEST_FRAME_BYTES + 1];
    let mut used = 0_usize;
    loop {
        let read = reader.read(&mut frame[used..]).map_err(|_| ())?;
        if read == 0 {
            return Err(());
        }
        let previous = used;
        used = used.checked_add(read).ok_or(())?;
        if let Some(relative) = frame[previous..used].iter().position(|byte| *byte == b'\n') {
            let newline = previous + relative;
            if newline != REQUEST_FRAME_BYTES - 1 || used != REQUEST_FRAME_BYTES {
                return Err(());
            }
            return parse_ping_request(&frame[..used]);
        }
        if used == frame.len() {
            return Err(());
        }
    }
}

fn parse_ping_request(frame: &[u8]) -> Result<[u8; PROTOCOL_ID_BYTES], ()> {
    if frame.len() != REQUEST_FRAME_BYTES
        || !frame.starts_with(REQUEST_PREFIX)
        || !frame.ends_with(REQUEST_SUFFIX)
    {
        return Err(());
    }
    let request_id_start = REQUEST_PREFIX.len();
    let request_id_end = request_id_start + PROTOCOL_ID_BYTES;
    let request_id_slice = &frame[request_id_start..request_id_end];
    if !is_protocol_id(request_id_slice) {
        return Err(());
    }
    let mut request_id = [0_u8; PROTOCOL_ID_BYTES];
    request_id.copy_from_slice(request_id_slice);
    Ok(request_id)
}

fn is_protocol_id(value: &[u8]) -> bool {
    value.len() == PROTOCOL_ID_BYTES
        && value
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
}

fn wait_for_host_termination() -> ! {
    loop {
        thread::park();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    const REQUEST_ID: &str = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

    #[test]
    fn accepts_only_the_deterministic_ping_frame() {
        let frame = format!("{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}\n");
        let request_id = read_ping_request(&mut Cursor::new(frame.as_bytes())).unwrap();
        assert_eq!(&request_id, REQUEST_ID.as_bytes());
    }

    #[test]
    fn rejects_unknown_duplicate_and_extra_fields() {
        let valid = format!("{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}\n");
        for frame in [
            format!("{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"health\"}}\n"),
            format!(
                "{{\"request_id\":\"{REQUEST_ID}\",\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}\n"
            ),
            format!(
                "{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\",\"extra\":true}}\n"
            ),
            format!("{valid}x"),
        ] {
            assert!(read_ping_request(&mut Cursor::new(frame.into_bytes())).is_err());
        }
    }

    #[test]
    fn rejects_oversize_incomplete_and_noncanonical_ids() {
        let oversize = vec![b'x'; REQUEST_FRAME_BYTES + 1];
        let incomplete = format!("{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}");
        let uppercase = format!(
            "{{\"request_id\":\"{}\",\"command\":\"ping\"}}\n",
            "A".repeat(PROTOCOL_ID_BYTES)
        );
        assert!(read_ping_request(&mut Cursor::new(oversize)).is_err());
        assert!(read_ping_request(&mut Cursor::new(incomplete.into_bytes())).is_err());
        assert!(read_ping_request(&mut Cursor::new(uppercase.into_bytes())).is_err());
    }
}
