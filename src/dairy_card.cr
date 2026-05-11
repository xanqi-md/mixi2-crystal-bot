require "http/client"
require "json"
require "uri"
require "set"
require "file_utils"

class CardCandidate
  include JSON::Serializable

  property name : String
  property image_url : String
  property source_url : String?

  def initialize(@name : String, @image_url : String, @source_url : String? = nil)
  end
end

class DailyCardPayload
  include JSON::Serializable

  property text : String
  property card_name : String
  property image_url : String
  property source_url : String?
  property date_key : String

  def initialize(@text : String, @card_name : String, @image_url : String, @source_url : String?, @date_key : String)
  end
end

class DailyCardSelector
  IMG_TAG_REGEX       = /<img\b[^>]*>/im
  ATTR_REGEX          = /([A-Za-z0-9:_-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/m
  DEFAULT_NAME_ATTRS  = ["data-card-name", "data-name", "alt", "title"] of String
  DEFAULT_IMAGE_ATTRS = ["data-src", "data-lazy-src", "data-original", "src"] of String

  def initialize
    @html_url = ENV["DAILY_CARD_HTML_URL"]?
    @html_file = ENV["DAILY_CARD_HTML_FILE"]?
    @image_base_url = ENV["DAILY_CARD_IMAGE_BASE_URL"]?
    @output_path = ENV["DAILY_CARD_PAYLOAD_PATH"]? || "/tmp/daily_card_payload.json"
    @post_label = ENV["DAILY_CARD_LABEL"]? || "今日の1枚"
  end

  def run
    html = load_html
    cards = extract_cards(html)

    if cards.empty?
      STDERR.puts "カード候補を抽出できませんでした。HTML 内の img alt/title/data-card-name を確認してください。"
      exit 1
    end

    date_key = jst_date_key
    chosen = select_card(cards, date_key)
    payload = DailyCardPayload.new(
      "#{@post_label}\n#{chosen.name}",
      chosen.name,
      chosen.image_url,
      chosen.source_url,
      date_key
    )

    FileUtils.mkdir_p(File.dirname(@output_path))
    File.write(@output_path, payload.to_json)

    puts "候補数: #{cards.size}"
    puts "選択日: #{date_key}"
    puts "カード名: #{chosen.name}"
    puts "画像URL: #{chosen.image_url}"
    puts "payload: #{@output_path}"
  end

  private def load_html : String
    if html_url = @html_url
      response = HTTP::Client.get(html_url, headers: HTTP::Headers{
        "User-Agent" => "mixi2-crystal-bot/1.0",
      })
      unless response.success?
        raise "HTML 取得失敗: #{response.status_code} #{response.status_message}"
      end
      return response.body
    end

    if html_file = @html_file
      return File.read(html_file)
    end

    raise "DAILY_CARD_HTML_URL または DAILY_CARD_HTML_FILE を指定してください"
  end

  private def extract_cards(html : String) : Array(CardCandidate)
    cards = [] of CardCandidate
    seen = Set(String).new

    offset = 0
    while match = IMG_TAG_REGEX.match(html, offset)
      tag = match[0]
      attrs = parse_attributes(tag)
      image_url = first_present(attrs, DEFAULT_IMAGE_ATTRS)
      name = first_present(attrs, DEFAULT_NAME_ATTRS)
      offset = match.end(0)

      next if image_url.nil? || image_url.to_s.empty?
      next if image_url.to_s.starts_with?("data:")
      next if name.nil? || name.to_s.strip.empty?

      normalized_name = clean_text(name.to_s)
      normalized_image_url = resolve_url(image_url.to_s)
      next if normalized_name.empty? || normalized_image_url.empty?

      key = "#{normalized_name}\u0000#{normalized_image_url}"
      next if seen.includes?(key)
      seen.add(key)

      cards << CardCandidate.new(normalized_name, normalized_image_url, @html_url || @html_file)
    end

    cards
  end

  private def parse_attributes(tag : String) : Hash(String, String)
    attrs = {} of String => String
    offset = 0

    while match = ATTR_REGEX.match(tag, offset)
      key = match[1].downcase
      value = match[2]? || match[3]? || match[4]? || ""
      attrs[key] = decode_html_entities(value)
      offset = match.end(0)
    end

    attrs
  end

  private def first_present(attrs : Hash(String, String), keys : Array(String)) : String?
    keys.each do |key|
      value = attrs[key]?
      return value unless value.nil? || value.empty?
    end
    nil
  end

  private def decode_html_entities(text : String) : String
    text
      .gsub("&amp;", "&")
      .gsub("&quot;", "\"")
      .gsub("&#39;", "'")
      .gsub("&apos;", "'")
      .gsub("&lt;", "<")
      .gsub("&gt;", ">")
      .gsub("&nbsp;", " ")
  end

  private def clean_text(text : String) : String
    decode_html_entities(text)
      .gsub(/<[^>]+>/, "")
      .gsub(/\s+/, " ")
      .strip
  end

  private def resolve_url(url : String) : String
    return url if url.starts_with?("http://") || url.starts_with?("https://")
    base = @image_base_url || @html_url
    return url unless base

    begin
      URI.parse(base).resolve(url).to_s
    rescue
      url
    end
  end

  private def jst_date_key : String
    now = Time.utc + 9.hours
    now.to_s("%Y-%m-%d")
  end

  private def select_card(cards : Array(CardCandidate), date_key : String) : CardCandidate
    seed = date_key.hash
    rng = Random.new(seed)
    cards[rng.rand(cards.size)]
  end
end

DailyCardSelector.new.run
