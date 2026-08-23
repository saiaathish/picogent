class Picogent < Formula
  desc "Tiny coding agent: two modes, BYOK or Ollama"
  homepage "https://github.com/saiaathish/picogent"
  url "https://github.com/saiaathish/picogent.git", branch: "main"
  version "1.0.0"
  license "MIT"
  head "https://github.com/saiaathish/picogent.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags), "./cmd/picogent"
  end

  test do
    assert_match "picogent", shell_output("#{bin}/picogent version")
  end
end
