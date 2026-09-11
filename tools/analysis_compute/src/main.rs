mod native_probe;

fn main() -> anyhow::Result<()> {
    let arguments: Vec<_> = std::env::args_os().skip(1).collect();
    if arguments
        .iter()
        .any(|argument| argument == native_probe::ARGUMENT)
    {
        if arguments.len() != 1 || arguments[0] != native_probe::ARGUMENT {
            std::process::exit(1);
        }
        if native_probe::run().is_err() {
            std::process::exit(1);
        }
        unreachable!("native probe waits for host termination")
    }
    analytix_analysis_compute::run_cli(std::env::args().skip(1))
}
